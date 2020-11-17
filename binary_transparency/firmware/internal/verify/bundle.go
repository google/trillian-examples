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

// The verify package holds helpers for validating the correctness of various
// artifacts and proofs used in the system.
package verify

import (
	"fmt"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
)

// Bundle checks that the manifest, checkpoint, and proofs in a bundle are all self-consistent.
func Bundle(b api.ProofBundle) error {
	lh := HashLeaf(b.ManifestStatement)
	lv := NewLogVerifier()

	if err := lv.VerifyInclusionProof(int64(b.InclusionProof.LeafIndex), int64(b.Checkpoint.TreeSize), b.InclusionProof.Proof, b.Checkpoint.RootHash, lh); err != nil {
		return fmt.Errorf("invalid inclusion proof in bundle: %w", err)
	}
	return nil
}
