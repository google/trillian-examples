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
	"sync"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/serverless/client"
	"golang.org/x/mod/sumdb/note"
)

// CheckpointNConsensus returns a ConsensusCheckpoint function which selects the newest checkpoint available from
// the available distributors which has signatures from at least N of the provided witnesses.
func CheckpointNConsensus(logID string, distributors []client.Fetcher, witnesses []note.Verifier, N int) client.ConsensusCheckpoint {
	// TODO(al): This implementation is pretty basic, and could be made better.
	// In particular, there are cases where it will fail to build consensus
	// where a more thorough algorithm would succeed.
	// e.g. currently, it will simply fetch  checkpoint.N from each of the
	// distributors and fail if either:
	//  - no distributor has a checkpoint.N file
	//  - no distributor has a checkpoint.N file signed by N of the known witnesses.
	//
	// It's good enough for now, though.
	return func(ctx context.Context, logSigV note.Verifier, origin string) (*log.Checkpoint, []byte, error) {
		type cp struct {
			cp  *log.Checkpoint
			n   *note.Note
			raw []byte
		}
		cpc := make(chan cp, len(distributors))
		wg := &sync.WaitGroup{}
		for _, f := range distributors {
			wg.Add(1)
			go func(ctx context.Context, f client.Fetcher, logID string, N int, cpc chan<- cp) {
				defer wg.Done()
				c, n, cpRaw, err := getCheckpointN(ctx, f, logID, N, logSigV, origin, witnesses)
				if err != nil {
					glog.Infof("Error talking to distributor: %v", err)
					return
				}
				cpc <- cp{
					cp:  c,
					n:   n,
					raw: cpRaw,
				}

			}(ctx, f, logID, N, cpc)
		}

		wg.Wait()
		close(cpc)

		glog.Infof("considering %d cps", len(cpc))

		var bestCP cp
		for cp := range cpc {
			if numWitSigs := len(cp.n.Sigs) - 1; numWitSigs < N {
				glog.V(1).Infof("Discarding CP with %d witness sigs, want at least %d", numWitSigs, N)
				continue
			}
			if bestCP.cp == nil || bestCP.cp.Size < cp.cp.Size {
				bestCP = cp
				continue
			}
		}
		if bestCP.cp == nil {
			return nil, nil, fmt.Errorf("unable to identify suitable checkpoint")
		}
		return bestCP.cp, bestCP.raw, nil

	}
}

func getCheckpointN(ctx context.Context, f client.Fetcher, logID string, N int, logSigV note.Verifier, origin string, witSigVs []note.Verifier) (*log.Checkpoint, *note.Note, []byte, error) {
	p := path.Join("logs", logID, fmt.Sprintf("checkpoint.%d", N))
	cpRaw, err := f(ctx, p)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to fetch from distributor: %v", err)
	}
	cp, _, n, err := log.ParseCheckpoint(cpRaw, origin, logSigV, witSigVs...)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error parsing Checkpoint from %q: %v", p, err)
	}
	return cp, n, cpRaw, nil
}
