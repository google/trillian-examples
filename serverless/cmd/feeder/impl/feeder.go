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
	Update(ctx context.Context, logID string, newCP []byte, proof [][]byte) error
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

	// NumRequired is the number of witness signatures required on checkpoint
	// (including any pre-existing signatures from known witnesses).
	NumRequired int

	// Witnesses is a list of witnesses to feed to.
	Witnesses []Witness

	// WitnessTimeout defines the maximum duration for each attempt to update a witness.
	// No timeout if unset.
	WitnessTimeout time.Duration
}

// Feed sends the provided checkpoint to the configured set of witnesses.
// This method will block until either opts.NumRequired witness signatures are obtained,
// or the context becomes done.
// Returns the provided checkpoint plus at least cfg.NumRequired signatures.
func Feed(ctx context.Context, cp []byte, opts FeedOpts) ([]byte, error) {
	if nw := len(opts.Witnesses); opts.NumRequired > nw {
		return nil, fmt.Errorf("NumRequired is %d, but only %d witnesses provided", opts.NumRequired, nw)
	}
	wSigVs := make([]note.Verifier, 0, len(opts.Witnesses))
	for _, w := range opts.Witnesses {
		wSigVs = append(wSigVs, w.SigVerifier())
	}

	have := make(map[uint32]bool)
	n, err := note.Open(cp, note.VerifierList(append([]note.Verifier{opts.LogSigVerifier}, wSigVs...)...))
	if err != nil {
		return nil, fmt.Errorf("failed to verify signature on checkpoint: %v", err)
	}
	for _, s := range n.Sigs {
		have[s.Hash] = true
	}
	if !have[opts.LogSigVerifier.KeyHash()] {
		return nil, errors.New("cp not signed by log")
	}
	cpSubmit := &log.Checkpoint{}
	_, err = cpSubmit.Unmarshal([]byte(n.Text))
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal CP: %v", err)
	}

	witnessed := make(chan []byte, len(opts.Witnesses))
	errs := make(chan error, len(opts.Witnesses))

	if opts.WitnessTimeout > 0 {
		var c func()
		ctx, c = context.WithTimeout(ctx, opts.WitnessTimeout)
		defer c()
	}
	for _, w := range opts.Witnesses {
		if have[w.SigVerifier().KeyHash()] {
			errs <- fmt.Errorf("skipping %q - already have their signature", w.SigVerifier().Name())
			continue
		}
		go submitToWitness(ctx, cp, *cpSubmit, w, opts.LogID, opts.LogFetcher, opts.LogSigVerifier, witnessed, errs)

	}

	// Gather results...
	allWitnessed := make([][]byte, 0, len(opts.Witnesses)+1)
	for range opts.Witnesses {
		select {
		case w := <-witnessed:
			allWitnessed = append(allWitnessed, w)
		case e := <-errs:
			glog.Warning(e.Error())
		case <-ctx.Done():
		}
	}

	if len(allWitnessed) == 0 {
		return cp, ErrNoSignaturesAdded
	}

	// ...and combine signatures.
	wvList := note.VerifierList(wSigVs...)
	r, err := checkpoints.Combine(append([][]byte{cp}, allWitnessed...), opts.LogSigVerifier, wvList)
	if err != nil {
		return nil, fmt.Errorf("failed to combine checkpoints: %v", err)
	}

	// Finally, check how many witness signatures we now have:
	n, err = note.Open(r, wvList)
	if err != nil {
		nErr, ok := err.(*note.UnverifiedNoteError)
		if !ok {
			return nil, fmt.Errorf("failed to open combined CP: %v", err)
		}
		n = nErr.Note
	}
	if got := len(n.Sigs); got < opts.NumRequired {
		return nil, fmt.Errorf("number of witness signatures (%d) < number required (%d)", got, opts.NumRequired)
	}

	return r, nil
}

// submitToWitness will keep trying to submit the checkpoint to the witness until the context is done.
func submitToWitness(ctx context.Context, cpRaw []byte, cpSubmit log.Checkpoint, w Witness, logID string, logFetcher client.Fetcher, logSigV note.Verifier, witnessed chan<- []byte, errs chan<- error) {
	// TODO(al): make this configurable
	h := rfc6962.DefaultHasher
	wSigV := w.SigVerifier()

	// Keep submitting until success or context timeout...
	t := time.NewTicker(1)
	for {
		select {
		case <-ctx.Done():
			errs <- fmt.Errorf("giving up on %s", wSigV.Name())
			return
		case <-t.C:
			t.Reset(time.Second)
		}

		latestCPRaw, err := w.GetLatestCheckpoint(ctx, logID)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			glog.Warningf("%s: failed to fetch latest CP: %v", wSigV.Name(), err)
			continue
		}

		var conP [][]byte
		if len(latestCPRaw) > 0 {
			n, err := note.Open(latestCPRaw, note.VerifierList(logSigV, wSigV))
			if err != nil {
				glog.Warningf("%s: failed to open CP: %v", wSigV.Name(), err)
				continue
			}
			if numSigs := len(n.Sigs); numSigs != 2 {
				errs <- errors.New("checkpoint from witness was not signed by at least log + witness")
				return
			}

			latestCP := &log.Checkpoint{}
			_, err = latestCP.Unmarshal([]byte(n.Text))
			if err != nil {
				glog.Warningf("%s: failed to unmarshal CP: %v", wSigV.Name(), err)
				continue
			}

			if latestCP.Size > cpSubmit.Size {
				errs <- fmt.Errorf("%s: witness checkpoint size (%d) > submit checkpoint size (%d)", wSigV.Name(), latestCP.Size, cpSubmit.Size)
				return
			}
			if latestCP.Size == cpSubmit.Size && bytes.Equal(latestCP.Hash, cpSubmit.Hash) {
				glog.V(1).Infof("got sig from witness: %v", wSigV.Name())
				witnessed <- latestCPRaw
				return
			}

			pb, err := client.NewProofBuilder(ctx, cpSubmit, h.HashChildren, logFetcher)
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

		if err := w.Update(ctx, logID, cpRaw, conP); err != nil {
			glog.Warningf("%s: failed to submit checkpoint to witness: %v", wSigV.Name(), err)
			continue
		}
	}
}
