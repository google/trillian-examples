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

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/crypto"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/verify"
	"github.com/google/trillian/merkle/logverifier"
)

type ErrConsistency struct {
	Golden, Latest api.LogCheckpoint
}

func (e ErrConsistency) Error() string {
	return fmt.Sprintf("failed to verify consistency proof from %s to %s", e.Golden, e.Latest)
}

type ErrInclusion struct {
	Checkpoint api.LogCheckpoint
	Proof      api.InclusionProof
}

func (e ErrInclusion) Error() string {
	return fmt.Sprintf("failed to verify inclusion proof %s in root %s", e.Proof, e.Checkpoint)
}

// LogEntry wraps up a leaf value with its position in the log.
type LogEntry struct {
	Root  api.LogCheckpoint
	Index uint64
	Value api.SignedStatement
}

// LogFollower follows a log for new data becoming available.
type LogFollower struct {
	c            ReadonlyClient
	lv           logverifier.LogVerifier
	pollInterval time.Duration
	golden       api.LogCheckpoint
}

func NewLogFollower(c ReadonlyClient, pollInterval time.Duration, golden api.LogCheckpoint) LogFollower {
	return LogFollower{
		c:            c,
		lv:           verify.NewLogVerifier(),
		pollInterval: pollInterval,
		golden:       golden,
	}
}

// Checkpoints polls the log according to the configured interval, returning roots consistent
// with the current golden checkpoint. Should any valid roots be found which are inconsistent
// then an error is returned. The log being unavailable will just cause a retry, giving it
// the benefit of the doubt.
func (f *LogFollower) Checkpoints(ctx context.Context) (<-chan api.LogCheckpoint, <-chan error) {
	ticker := time.NewTicker(f.pollInterval)

	outc := make(chan api.LogCheckpoint, 1)
	errc := make(chan error, 1)

	// Push the starting checkpoint into the channel here to kick off the pipeline
	outc <- f.golden

	go func() {
		defer close(outc)
		// Now keep looking for newer, consistent checkpoints
		for {
			select {
			case <-ticker.C:
				// Wait until the next tick.
			case <-ctx.Done():
				errc <- ctx.Err()
				return
			}

			cp, err := f.c.GetCheckpoint()
			if err != nil {
				glog.Warningf("Failed to get latest Checkpoint: %q", err)
				continue
			}
			// TODO(al): check signature on checkpoint when they're added.

			if cp.TreeSize <= f.golden.TreeSize {
				continue
			}
			glog.V(1).Infof("Got newer checkpoint %s", cp)

			// Perform consistency check only for non-zero initial tree size
			if f.golden.TreeSize != 0 {
				consistency, err := f.c.GetConsistencyProof(api.GetConsistencyRequest{From: f.golden.TreeSize, To: cp.TreeSize})
				if err != nil {
					glog.Warningf("Failed to fetch the Consistency: %q", err)
					continue
				}
				glog.V(1).Infof("Printing the latest Consistency Proof Information")
				glog.V(1).Infof("Consistency Proof = %x", consistency.Proof)

				// Verify the fetched consistency proof
				if err := f.lv.VerifyConsistencyProof(int64(f.golden.TreeSize), int64(cp.TreeSize), f.golden.RootHash, cp.RootHash, consistency.Proof); err != nil {
					errc <- ErrConsistency{
						Golden: f.golden,
						Latest: *cp,
					}
					return
				}
				glog.V(1).Infof("Consistency proof for Treesize %d verified", cp.TreeSize)
			}
			f.golden = *cp
			outc <- *cp
		}
	}()
	return outc, errc
}

func (f *LogFollower) Entries(ctx context.Context, cpc <-chan api.LogCheckpoint, head uint64) (<-chan LogEntry, <-chan error) {
	outc := make(chan LogEntry, 1)
	errc := make(chan error, 1)

	go func() {
		defer close(outc)
		for cp := range cpc {
			for ; head < cp.TreeSize; head++ {
				proof, err := f.c.GetManifestEntryAndProof(api.GetFirmwareManifestRequest{Index: head, TreeSize: cp.TreeSize})
				if err != nil {
					glog.Warningf("Failed to fetch the Manifest: %q", err)
					continue
				}
				lh := verify.HashLeaf(proof.Value)
				if err := f.lv.VerifyInclusionProof(int64(proof.LeafIndex), int64(cp.TreeSize), proof.Proof, cp.RootHash, lh); err != nil {
					errc <- ErrInclusion{
						Checkpoint: cp,
						Proof:      *proof,
					}
					return
				}
				glog.V(1).Infof("Inclusion proof for leafhash 0x%x verified", lh)

				statement := proof.Value
				stmt := api.SignedStatement{}
				if err := json.NewDecoder(bytes.NewReader(statement)).Decode(&stmt); err != nil {
					errc <- fmt.Errorf("failed to decode SignedStatement: %q", err)
					return
				}

				claimant, err := crypto.ClaimantForType(stmt.Type)
				if err != nil {
					errc <- err
					return
				}
				// Verify the signature:
				if err := claimant.VerifySignature(stmt.Type, stmt.Statement, stmt.Signature); err != nil {
					errc <- fmt.Errorf("failed to verify signature: %q", err)
					return
				}
				outc <- LogEntry{
					Root:  cp,
					Index: head,
					Value: stmt,
				}
			}
		}
	}()
	return outc, errc
}
