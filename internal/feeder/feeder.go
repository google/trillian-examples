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

// feeder provides support for building witness feeder implementations.
package feeder

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/formats/log"
	"golang.org/x/mod/sumdb/note"
)

// ErrNoSignaturesAdded is returned when the witness has already signed the presented checkpoint.
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

	// FetchProof should return a consistency proof from the source log.
	//
	// Note that if the witness knows the log but has no previous checkpoint stored, this
	// function will be called with a default `from` value - this allows compact-range
	// type proofs to be supported.  Implementations for non-compact-range type proofs
	// should return an empty proof and no error.
	FetchProof func(ctx context.Context, from, to log.Checkpoint) ([][]byte, error)

	// LogSigVerifier a verifier for log checkpoint signatures.
	LogSigVerifier note.Verifier
	// LogOrigin is the expected first line of checkpoints from the source log.
	LogOrigin string

	Witness Witness

	// WitnessTimeout defines the maximum duration for each attempt to update a witness.
	// No timeout if unset.
	WitnessTimeout time.Duration
}

// Feed sends the provided checkpoint to the configured witness.
// This method will block until a witness signature is obtained,
// or the context becomes done.
func Feed(ctx context.Context, cp []byte, opts FeedOpts) ([]byte, error) {
	cpSubmit, _, n, err := log.ParseCheckpoint(cp, opts.LogOrigin, opts.LogSigVerifier, opts.Witness.SigVerifier())
	if err != nil {
		return nil, fmt.Errorf("failed to parse checkpoint: %v", err)
	}
	if len(n.Sigs) == 2 {
		return cp, ErrNoSignaturesAdded
	}
	if opts.WitnessTimeout > 0 {
		var c func()
		ctx, c = context.WithTimeout(ctx, opts.WitnessTimeout)
		defer c()
	}
	wCP, err := submitToWitness(ctx, cp, *cpSubmit, opts)
	if err != nil {
		return nil, fmt.Errorf("witness submission failed: %v", err)
	}
	return wCP, nil
}

// submitToWitness will keep trying to submit the checkpoint to the witness until the context is done.
func submitToWitness(ctx context.Context, cpRaw []byte, cpSubmit log.Checkpoint, opts FeedOpts) ([]byte, error) {
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
		var latestCP log.Checkpoint
		if len(latestCPRaw) > 0 {
			cp, _, n, err := log.ParseCheckpoint(latestCPRaw, opts.LogOrigin, opts.LogSigVerifier, wSigV)
			if err != nil {
				glog.Warningf("%s: failed to parse CP: %v", wSigV.Name(), err)
				continue
			}
			latestCP = *cp

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
		}

		// The witness may be configured to expect a compact-range type proof, so we need to always
		// try to build one, even if the witness doesn't have a "latest" checkpoint for this log.
		conP, err = opts.FetchProof(ctx, latestCP, cpSubmit)
		if err != nil {
			glog.Warningf("%s: failed to fetch consistency proof: %v", wSigV.Name(), err)
			continue
		}

		if cp, err := opts.Witness.Update(ctx, opts.LogID, cpRaw, conP); err != nil {
			glog.Warningf("%s: failed to submit checkpoint to witness: %v", wSigV.Name(), err)
			continue
		} else {
			return cp, nil
		}
	}
}
