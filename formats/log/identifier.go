package log

import (
	"crypto/sha256"
	"fmt"
)

// ID returns the identifier to use for a log given the Origin
// and the public key. This is the ID used to find checkpoints
// for this log at distributors, and that will be used to feed
// checkpoints to witnesses.
func ID(origin string, key []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(key))
}
