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
	"github.com/transparency-dev/merkle"
)

// ErrConsistency is returned if two logs roots are found which are inconsistent.
// This allows a motivated client to extract the checkpoints to provide as evidence
// if needed.
type ErrConsistency struct {
	Golden, Latest api.LogCheckpoint
}

func (e ErrConsistency) Error() string {
	return fmt.Sprintf("failed to verify consistency proof from %s to %s", e.Golden, e.Latest)
}

// ErrInclusion is returned if a proof of inclusion does not validate.
// This allows a motivated client to provide evidence if needed.
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
	c  ReadonlyClient
	lv merkle.LogVerifier
}

// NewLogFollower creates a LogFollower that uses the given client.
func NewLogFollower(c ReadonlyClient) LogFollower {
	return LogFollower{
		c:  c,
		lv: verify.NewLogVerifier(),
	}
}

// Checkpoints polls the log according to the configured interval, returning roots consistent
// with the current golden checkpoint. Should any valid roots be found which are inconsistent
// then an error is returned. The log being unavailable will just cause a retry, giving it
// the benefit of the doubt.
func (f *LogFollower) Checkpoints(ctx context.Context, pollInterval time.Duration, golden api.LogCheckpoint) (<-chan api.LogCheckpoint, <-chan error) {
	ticker := time.NewTicker(pollInterval)

	outc := make(chan api.LogCheckpoint, 1)
	errc := make(chan error, 1)

	// Push the starting checkpoint into the channel here to kick off the pipeline
	outc <- golden

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

			if cp.Size <= golden.Size {
				continue
			}
			glog.V(1).Infof("Got newer checkpoint %s", cp)

			// Perform consistency check only for non-zero initial tree size
			if golden.Size != 0 {
				consistency, err := f.c.GetConsistencyProof(api.GetConsistencyRequest{From: golden.Size, To: cp.Size})
				if err != nil {
					glog.Warningf("Failed to fetch the Consistency: %q", err)
					continue
				}
				glog.V(1).Infof("Printing the latest Consistency Proof Information")
				glog.V(1).Infof("Consistency Proof = %x", consistency.Proof)

				// Verify the fetched consistency proof
				if err := f.lv.VerifyConsistency(golden.Size, cp.Size, golden.Hash, cp.Hash, consistency.Proof); err != nil {
					errc <- ErrConsistency{
						Golden: golden,
						Latest: *cp,
					}
					return
				}
				glog.V(1).Infof("Consistency proof for Treesize %d verified", cp.Size)
			}
			golden = *cp
			outc <- *cp
		}
	}()
	return outc, errc
}

// Entries follows the log to output all of the leaves starting from the head index provided.
// This is intended to be set up to consume the output of #Checkpoints(), and will output new
// entries each time a Checkpoint becomes available which is larger than the current head.
// The input channel should be closed in order to clean up the resources used by this method.
func (f *LogFollower) Entries(ctx context.Context, cpc <-chan api.LogCheckpoint, head uint64) (<-chan LogEntry, <-chan error) {
	outc := make(chan LogEntry, 1)
	errc := make(chan error, 1)

	go func() {
		defer close(outc)
		for cp := range cpc {
			for ; head < cp.Size; head++ {
				proof, err := f.c.GetManifestEntryAndProof(api.GetFirmwareManifestRequest{Index: head, TreeSize: cp.Size})
				if err != nil {
					glog.Warningf("Failed to fetch the Manifest: %q", err)
					continue
				}
				lh := verify.HashLeaf(proof.Value)
				if err := f.lv.VerifyInclusion(proof.LeafIndex, cp.Size, lh, proof.Proof, cp.Hash); err != nil {
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
