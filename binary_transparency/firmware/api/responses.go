package api

import "fmt"

// LogCheckpoint commits to the state of the log.
// TODO(mhutchinson): This needs a signature to be worth anything, which
// requires a known serialization. This will be changed in the future but works
// well enough for the state of the demo at this time.
type LogCheckpoint struct {
	TreeSize uint64
	RootHash []byte
	// The number of nanoseconds since the Unix epoch.
	TimestampNanos uint64
}

// String returns a compact printable representation of a LogCheckpoint.
func (l LogCheckpoint) String() string {
	return fmt.Sprintf("{size %d @ %d root: 0x%x}", l.TreeSize, l.TimestampNanos, l.RootHash)
}
