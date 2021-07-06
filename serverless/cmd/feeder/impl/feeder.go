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
	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/serverless/client"
	"github.com/google/trillian/merkle/rfc6962/hasher"
	"golang.org/x/mod/sumdb/note"
)

type Witness interface {
	SigVerifier() note.Verifier
	GetLatestCheckpoint(ctx context.Context, logID string) ([]byte, error)
	Update(ctx context.Context, logID string, newCP []byte, proof [][]byte) error
}

type FeedOpts struct {
	LogID          string
	LogFetcher     client.Fetcher
	LogSigVerifier note.Verifier

	NumRequired int

	Witnesses []Witness
}

// Feed sends the provided checkpoint to the configured set of witnesses.
// Returns the provided checkpoint plus at least cfg.NumRequired signatures.
func Feed(ctx context.Context, cp []byte, opts FeedOpts) ([]byte, error) {
	n, err := note.Open(cp, note.VerifierList(opts.LogSigVerifier))
	if err != nil {
		return nil, fmt.Errorf("failed to verify signature on checkpoint: %v", err)
	}
	cpSubmit := &log.Checkpoint{}
	_, err = cpSubmit.Unmarshal([]byte(n.Text))
	if err != nil {
		return nil, fmt.Errorf("failed to verify signature on checkpoint to witness: %v", err)
	}

	sigs := make(chan note.Signature, len(opts.Witnesses))
	errs := make(chan error, len(opts.Witnesses))
	// TODO(al): make this configurable
	h := hasher.DefaultHasher

	for _, w := range opts.Witnesses {
		go func(ctx context.Context, w Witness) {
			// Keep submitting until success or context timeout...
			t := time.NewTicker(1)
			wSigV := w.SigVerifier()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					t.Reset(5 * time.Second)
				}

				latestCPRaw, err := w.GetLatestCheckpoint(ctx, opts.LogID)
				if err != nil && !errors.Is(err, os.ErrNotExist) {
					glog.Warningf("%s: failed to fetch latest CP: %v", wSigV.Name(), err)
					continue
				}

				var conP [][]byte
				if len(latestCPRaw) > 0 {
					n, err := note.Open(latestCPRaw, note.VerifierList(wSigV))
					if err != nil {
						glog.Warningf("%s: failed to open CP: %v", wSigV.Name(), err)
						continue
					}

					latestCP := &log.Checkpoint{}
					_, err = latestCP.Unmarshal(latestCPRaw)
					if err != nil {
						glog.Warningf("%s: failed to unmarshal CP: %v", wSigV.Name(), err)
						continue
					}

					if latestCP.Size > cpSubmit.Size {
						errs <- fmt.Errorf("%s: witness checkpoint size (%d) > submit checkpoint size (%d)", wSigV.Name(), latestCP.Size, cpSubmit.Size)
						return
					}
					if latestCP.Size == cpSubmit.Size && bytes.Equal(latestCP.Hash, cpSubmit.Hash) {
						sigs <- n.Sigs[0]
						return
					}

					pb, err := client.NewProofBuilder(ctx, *cpSubmit, h.HashChildren, opts.LogFetcher)
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

				if err := w.Update(ctx, opts.LogID, cp, conP); err != nil {
					glog.Warningf("%s: failed to submit checkpoint to witness: %v", wSigV.Name(), err)
					continue
				}

			}
		}(ctx, w)
	}

	for range opts.Witnesses {
		select {
		case s := <-sigs:
			n.Sigs = append(n.Sigs, s)
		case e := <-errs:
			glog.Warning(e.Error())
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if got := len(n.Sigs); got < opts.NumRequired {
		return nil, fmt.Errorf("number of witness signatures (%d) < number required (%d)", got, opts.NumRequired)
	}
	return note.Sign(n)
}
