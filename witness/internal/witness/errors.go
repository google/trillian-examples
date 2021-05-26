
package witness

import (
	"fmt"

	"github.com/google/trillian-examples/formats/log"
)

// ErrInconsistency should be returned when there has been an error proving
// consistency between two checkpoints.
type ErrInconsistency struct {
	Smaller []byte
	Larger  []byte
	Proof   log.Proof

	Wrapped error
}

func (e ErrInconsistency) Unwrap() error {
	return e.Wrapped
}

func (e ErrInconsistency) Error() string {
	return fmt.Sprintf("log consistency check failed: %s", e.Wrapped)
}

// ErrStale should be returned when Update has been called with `from` set to
// something other than the latest checkpoint.
type ErrStale struct {
	Smaller *log.Checkpoint
	Latest  *log.Checkpoint
}

func (e ErrStale) Error() string {
	return fmt.Sprintf("stale checkpoint given as input")
}
