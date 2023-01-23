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

// Package witness provides client support for using witnesses.
package witness

import (
	"context"
	"fmt"
	"path"

	"github.com/google/trillian-examples/serverless/client"
	"golang.org/x/mod/sumdb/note"
	"golang.org/x/sync/errgroup"

	fmt_log "github.com/transparency-dev/formats/log"
)

// CheckpointNConsensus returns a ConsensusCheckpoint function which selects the newest checkpoint available from
// the available distributors which has signatures from at least N of the provided witnesses.
func CheckpointNConsensus(logID string, distributors []client.Fetcher, witnesses []note.Verifier, N int) (client.ConsensusCheckpointFunc, error) {

	if nw := len(witnesses); N > nw {
		return nil, fmt.Errorf("requested consensus across %d witnesses, but only %d witnesses configured - consensus would always fail", N, nw)
	}

	// TODO(al): This implementation is pretty basic, and could be made better.
	// In particular, there are cases where it will fail to build consensus
	// where a more thorough algorithm would succeed.
	// e.g. currently, it will simply fetch checkpoint.N from each of the
	// distributors and fail if either:
	//  - no distributor has a checkpoint.N file
	//  - no distributor has a checkpoint.N file signed by N of the known witnesses.
	//
	// It's good enough for now, though.
	return func(ctx context.Context, logSigV note.Verifier, origin string) (*fmt_log.Checkpoint, []byte, *note.Note, error) {
		type cp struct {
			cp  *fmt_log.Checkpoint
			n   *note.Note
			raw []byte
		}
		cpc := make(chan cp, len(distributors))
		eg, ctx := errgroup.WithContext(ctx)
		for _, f := range distributors {
			fn := func(ctx context.Context, f client.Fetcher, logID string, N int, logSigV note.Verifier, origin string, witnesses []note.Verifier, cpc chan<- cp) func() error {
				return func() error {
					c, n, cpRaw, err := getCheckpointN(ctx, f, logID, N, logSigV, origin, witnesses)
					if err != nil {
						return fmt.Errorf("error talking to distributor: %v", err)
					}
					cpc <- cp{
						cp:  c,
						n:   n,
						raw: cpRaw,
					}
					return nil
				}
			}
			eg.Go(fn(ctx, f, logID, N, logSigV, origin, witnesses, cpc))
		}

		fetchErrs := eg.Wait()
		close(cpc)

		var bestCP cp
		for c := range cpc {
			if numWitSigs := len(c.n.Sigs) - 1; numWitSigs < N {
				continue
			}
			if bestCP.cp == nil || bestCP.cp.Size < c.cp.Size {
				bestCP = c
				continue
			}
		}
		if bestCP.cp == nil {
			return nil, nil, nil, fmt.Errorf("unable to identify suitable checkpoint (fetch errs: %v)", fetchErrs)
		}
		return bestCP.cp, bestCP.raw, bestCP.n, nil
	}, nil
}

func getCheckpointN(ctx context.Context, f client.Fetcher, logID string, N int, logSigV note.Verifier, origin string, witSigVs []note.Verifier) (*fmt_log.Checkpoint, *note.Note, []byte, error) {
	p := path.Join("logs", logID, fmt.Sprintf("checkpoint.%d", N))
	cpRaw, err := f(ctx, p)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to fetch from distributor: %v", err)
	}
	cp, _, n, err := fmt_log.ParseCheckpoint(cpRaw, origin, logSigV, witSigVs...)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error parsing Checkpoint from %q: %v", p, err)
	}
	return cp, n, cpRaw, nil
}
