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

// Package feeder provides support for building witness feeder implementations.
package feeder

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/golang/glog"
	"github.com/transparency-dev/formats/log"
	"golang.org/x/mod/sumdb/note"
)

// ErrNoSignaturesAdded is returned when the witness has already signed the presented checkpoint.
var ErrNoSignaturesAdded = errors.New("no additional signatures added")

// Witness describes the operations the feeder needs to interact with a witness.
type Witness interface {
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

	// FetchCheckpoint should return a recent checkpoint from the source log.
	FetchCheckpoint func(ctx context.Context) ([]byte, error)

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
}

// FeedOnce sends the provided checkpoint to the configured witness.
// This method will block until a witness signature is obtained,
// or the context becomes done.
func FeedOnce(ctx context.Context, opts FeedOpts) ([]byte, error) {
	cp, err := opts.FetchCheckpoint(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read input checkpoint: %v", err)
	}

	glog.V(2).Infof("CP to feed:\n%s", string(cp))

	cpSubmit, _, _, err := log.ParseCheckpoint(cp, opts.LogOrigin, opts.LogSigVerifier)
	if err != nil {
		return nil, fmt.Errorf("failed to parse checkpoint: %v", err)
	}

	wCP, err := submitToWitness(ctx, cp, *cpSubmit, opts)
	if err != nil {
		return nil, fmt.Errorf("witness submission failed: %v", err)
	}
	return wCP, nil
}

// Run periodically initiates a feed cycle, fetching a checkpoint from the source log and
// submitting it to the witness.
// Calling this function will block until the context is done.
func Run(ctx context.Context, interval time.Duration, opts FeedOpts) error {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		// Create a scope with a bounded context so we don't get wedged if something goes wrong.
		func() {
			ctx, cancel := context.WithTimeout(ctx, interval)
			defer cancel()

			if _, err := FeedOnce(ctx, opts); err != nil {
				glog.Errorf("Feeding log %q failed: %v", opts.LogSigVerifier.Name(), err)
			}
		}()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// submitToWitness will keep trying to submit the checkpoint to the witness until the context is done.
func submitToWitness(ctx context.Context, cpRaw []byte, cpSubmit log.Checkpoint, opts FeedOpts) ([]byte, error) {
	var returnCp []byte

	// Since this func will be executed by the backoff mechanism below, we'll
	// log any error messages directly in here before returning the error, as
	// the backoff util doesn't seem to log them itself.
	submitOp := func() error {
		logName := opts.LogSigVerifier.Name()
		latestCPRaw, err := opts.Witness.GetLatestCheckpoint(ctx, opts.LogID)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			e := fmt.Errorf("failed to fetch latest CP from witness: %v", err)
			glog.Warning(e.Error())
			return e
		}

		var conP [][]byte
		var latestCP log.Checkpoint
		if len(latestCPRaw) > 0 {
			cp, _, _, err := log.ParseCheckpoint(latestCPRaw, opts.LogOrigin, opts.LogSigVerifier)
			if err != nil {
				e := fmt.Errorf("failed to parse CP from witness: %v", err)
				glog.Warning(e.Error())
				return e
			}
			latestCP = *cp

			if latestCP.Size > cpSubmit.Size {
				return backoff.Permanent(fmt.Errorf("witness checkpoint size (%d) > submit checkpoint size (%d)", latestCP.Size, cpSubmit.Size))
			}
			if latestCP.Size == cpSubmit.Size && bytes.Equal(latestCP.Hash, cpSubmit.Hash) {
				glog.V(1).Infof("%q unchanged - @%d: %x", logName, latestCP.Size, latestCP.Hash)
				returnCp = latestCPRaw
				return nil
			}
		}

		glog.V(1).Infof("%q grew - @%d: %x → @%d: %x", logName, latestCP.Size, latestCP.Hash, cpSubmit.Size, cpSubmit.Hash)

		// The witness may be configured to expect a coct-range type proof, so we need to always
		// try to build one, even if the witness doesn't have a "latest" checkpoint for this log.
		conP, err = opts.FetchProof(ctx, latestCP, cpSubmit)
		if err != nil {
			e := fmt.Errorf("failed to fetch consistency proof: %v", err)
			glog.Warning(e.Error())
			return e
		}
		glog.V(2).Infof("%q %d -> %d proof: %x", logName, latestCP.Size, cpSubmit.Size, conP)

		if returnCp, err = opts.Witness.Update(ctx, opts.LogID, cpRaw, conP); err != nil {
			e := fmt.Errorf("failed to submit checkpoint to witness: %v", err)
			glog.Warning(e.Error())
			return e
		}
		return nil
	}

	err := backoff.Retry(submitOp, backoff.WithContext(backoff.NewExponentialBackOff(), ctx))
	return returnCp, err
}
