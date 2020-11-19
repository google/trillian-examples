// Copyright 2020 Google LLC. All Rights Reserved.
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
	"context"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/verify"
)

// AwaitInclusion waits for the specified statement s to be included into the log and then
// returns the checkpoint under which it was found to be present, along with valid consistency and inclusion proofs.
func AwaitInclusion(ctx context.Context, c *ReadonlyClient, cp api.LogCheckpoint, s []byte) (api.LogCheckpoint, api.ConsistencyProof, api.InclusionProof, error) {
	lh := verify.HashLeaf(s)
	lv := verify.NewLogVerifier()
	for {
		select {
		case <-time.After(1 * time.Second):
			//
		case <-ctx.Done():
			return api.LogCheckpoint{}, api.ConsistencyProof{}, api.InclusionProof{}, ctx.Err()
		}

		newCP, err := c.GetCheckpoint()
		if err != nil {
			return api.LogCheckpoint{}, api.ConsistencyProof{}, api.InclusionProof{}, err
		}
		// TODO(al): check signature on checkpoint when they're added.

		if newCP.TreeSize <= cp.TreeSize {
			glog.V(1).Info("Waiting for tree to integrate new leaves")
			continue
		}
		var consistency api.ConsistencyProof
		if cp.TreeSize > 0 {
			cproof, err := c.GetConsistencyProof(api.GetConsistencyRequest{From: cp.TreeSize, To: newCP.TreeSize})
			if err != nil {
				glog.Warningf("Received error while fetching consistency proof: %q", err)
				continue
			}
			consistency = *cproof
			if err := lv.VerifyConsistencyProof(int64(cp.TreeSize), int64(newCP.TreeSize), cp.RootHash, newCP.RootHash, consistency.Proof); err != nil {
				// Whoa Nelly, this is bad - bail!
				glog.Warning("Invalid consistency proof received!")
				return cp, consistency, api.InclusionProof{}, fmt.Errorf("invalid inclusion proof received: %w", err)
			}
			glog.Infof("Consistency proof between %d and %d verified", cp.TreeSize, newCP.TreeSize)
		}
		cp = *newCP

		ip, err := c.GetInclusion(s, cp)
		if err != nil {
			glog.Warningf("Received error while fetching inclusion proof: %q", err)
			continue
		}
		if err := lv.VerifyInclusionProof(int64(ip.LeafIndex), int64(cp.TreeSize), ip.Proof, cp.RootHash, lh); err != nil {
			// Whoa Nelly, this is bad - bail!
			glog.Warning("Invalid inclusion proof received!")
			return cp, consistency, ip, fmt.Errorf("invalid inclusion proof received: %w", err)
		}

		glog.Infof("Inclusion proof for leafhash 0x%x verified", lh)
		return cp, consistency, ip, nil
	}
	// unreachable
}
