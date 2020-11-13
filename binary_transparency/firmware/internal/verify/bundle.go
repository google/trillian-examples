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
