package verify

import (
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/rfc6962"
)

// NewLogVerifier is a convenience function to return a log verifier suitable for use
// with the FT personality.
func NewLogVerifier() merkle.LogVerifier {
	return merkle.NewLogVerifier(rfc6962.DefaultHasher)
}

// HashLeaf is a convenience function to return a leaf hash for a given leaf.
func HashLeaf(leaf []byte) []byte {
	return rfc6962.DefaultHasher.HashLeaf(leaf)
}
