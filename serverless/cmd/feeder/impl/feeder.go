// Copyright 2021 Google LLC. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// impl is a witness feeder implementation for the serverless log.
package impl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/formats/checkpoints"
	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/serverless/client"
	"github.com/google/trillian/merkle/rfc6962"
	"golang.org/x/mod/sumdb/note"
)

// ErrNoSignaturesAdded is returned when all known witnesses have already signed the presented checkpoint.
var ErrNoSignaturesAdded = errors.New("no additional signatures added")

// Witness describes the operations the feeder needs to interact with a witness.
type Witness interface {
	// SigVerifier returns a verifier which can check signatures from this witness.
	SigVerifier() note.Verifier
	// GetLatestCheckpoint returns the latest checkpoint the witness holds for the given logID.
	// Must return os.ErrNotExists if the logID is known, but it has no checkpoint for that log.
	GetLatestCheckpoint(ctx context.Context, logID string) ([]byte, error)
	// Update attempts to clock the witness forward for the given logID.
	// The latest signed checkpoint will be returned if this succeeds, or if the error is
	// http.ErrCheckpointTooOld. In all other cases no checkpoint should be expected.
	Update(ctx context.Context, logID string, newCP []byte, proof [][]byte) ([]byte, error)
}

// FeedOpts holds parameters when calling the Feed function.
type FeedOpts struct {
	// LogID is the ID for the log whose checkpoint is being fed.
	//
	// TODO(al/mhutchinson): should this be an impl detail of Witness
	// rather than present here just to be passed back in to Witness calls?
	LogID string
	// LogFetcher for the source log (used to build proofs).
	LogFetcher client.Fetcher
	// LogSigVerifier a verifier for log checkpoint signatures.
	LogSigVerifier note.Verifier
	// LogOrigin is the expected first line of checkpoints from the source log.
	LogOrigin string

	Witness Witness

	// WitnessTimeout defines the maximum duration for each attempt to update a witness.
	// No timeout if unset.
	WitnessTimeout time.Duration
}

// Feed sends the provided checkpoint to the configured set of witnesses.
// This method will block until either opts.NumRequired witness signatures are obtained,
// or the context becomes done.
// Returns the provided checkpoint plus at least cfg.NumRequired signatures.
func Feed(ctx context.Context, cp []byte, opts FeedOpts) ([]byte, error) {
	have := make(map[uint32]bool)

	cpSubmit, _, n, err := log.ParseCheckpoint(cp, opts.LogOrigin, opts.LogSigVerifier, opts.Witness.SigVerifier())
	if err != nil {
		return nil, fmt.Errorf("failed to parse checkpoint: %v", err)
	}
	for _, s := range n.Sigs {
		have[s.Hash] = true
	}
	if !have[opts.LogSigVerifier.KeyHash()] {
		return nil, errors.New("cp not signed by log")
	}

	if opts.WitnessTimeout > 0 {
		var c func()
		ctx, c = context.WithTimeout(ctx, opts.WitnessTimeout)
		defer c()
	}
	if have[opts.Witness.SigVerifier().KeyHash()] {
		return nil, ErrNoSignaturesAdded
	}
	wCP, err := submitToWitness(ctx, cp, *cpSubmit, opts)
	if err != nil {
		return nil, fmt.Errorf("witness submission failed: %v", err)
	}

	// ...and combine signatures.
	r, err := checkpoints.Combine(append([][]byte{cp}, wCP), opts.LogSigVerifier, note.VerifierList(opts.Witness.SigVerifier()))
	if err != nil {
		return nil, fmt.Errorf("failed to combine checkpoints: %v", err)
	}

	return r, nil
}

// submitToWitness will keep trying to submit the checkpoint to the witness until the context is done.
func submitToWitness(ctx context.Context, cpRaw []byte, cpSubmit log.Checkpoint, opts FeedOpts) ([]byte, error) {
	// TODO(al): make this configurable
	h := rfc6962.DefaultHasher
	wSigV := opts.Witness.SigVerifier()

	// Keep submitting until success or context timeout...
	t := time.NewTicker(1)
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("giving up on %s", wSigV.Name())
		case <-t.C:
			t.Reset(time.Second)
		}

		latestCPRaw, err := opts.Witness.GetLatestCheckpoint(ctx, opts.LogID)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			glog.Warningf("%s: failed to fetch latest CP: %v", wSigV.Name(), err)
			continue
		}

		var conP [][]byte
		if len(latestCPRaw) > 0 {
			latestCP, _, n, err := log.ParseCheckpoint(latestCPRaw, opts.LogOrigin, opts.LogSigVerifier, wSigV)
			if err != nil {
				glog.Warningf("%s: failed to parse CP: %v", wSigV.Name(), err)
				continue
			}
			if numSigs := len(n.Sigs); numSigs != 2 {
				return nil, errors.New("checkpoint from witness was not signed by at least log + witness")
			}

			if latestCP.Size > cpSubmit.Size {
				return nil, fmt.Errorf("%s: witness checkpoint size (%d) > submit checkpoint size (%d)", wSigV.Name(), latestCP.Size, cpSubmit.Size)
			}
			if latestCP.Size == cpSubmit.Size && bytes.Equal(latestCP.Hash, cpSubmit.Hash) {
				glog.V(1).Infof("got sig from witness: %v", wSigV.Name())
				return latestCPRaw, nil
			}

			pb, err := client.NewProofBuilder(ctx, cpSubmit, h.HashChildren, opts.LogFetcher)
			if err != nil {
				glog.Warningf("%s: failed to create proof builder: %v", wSigV.Name(), err)
				continue
			}

			conP, err = pb.ConsistencyProof(ctx, latestCP.Size, cpSubmit.Size)
			if err != nil {
				glog.Warningf("%s: failed to build consistency proof: %v", wSigV.Name(), err)
				continue
			}
		}

		// TODO(mhutchinson): This returns the checkpoint, which can be used instead of getting
		// the latest each time around the loop.
		if _, err := opts.Witness.Update(ctx, opts.LogID, cpRaw, conP); err != nil {
			glog.Warningf("%s: failed to submit checkpoint to witness: %v", wSigV.Name(), err)
			continue
		}
	}
}
