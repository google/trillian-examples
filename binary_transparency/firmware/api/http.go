package api

import "fmt"

const (
	// HTTPAddFirmware is the path of the URL to publish a firmware entry.
	HTTPAddFirmware = "ft/v0/add-firmware"
	// HTTPGetConsistency is the path of the URL to get a consistency proof between log roots.
	HTTPGetConsistency = "ft/v0/get-consistency"
	// HTTPGetInclusion is the path of the URL to get inclusion proofs for entries in the log.
	HTTPGetInclusion = "ft/v0/get-inclusion"
	// HTTPGetManifestEntryAndProof is the path of the URL to get firmware manifest entries with inclusion proofs.
	HTTPGetManifestEntryAndProof = "ft/v0/get-firmware-manifest-entry-and-proof"
	// HTTPGetRoot is the path of the URL to get a recent log root.
	HTTPGetRoot = "ft/v0/get-root"
)

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

// GetConsistencyRequest is sent to ask for a proof that the tree at ToSize
// is append-only from the tree at FromSize. The response is a ConsistencyProof.
type GetConsistencyRequest struct {
	From uint64
	To   uint64
}

// ConsistencyProof contains the hashes to demonstrate the append-only evolution
// of the log.
type ConsistencyProof struct {
	Proof [][]byte
}

// GetFirmwareManifestRequest is sent to ask for the value at the given LeafIndex,
// with an inclusion proof to the root at the given TreeSize.
type GetFirmwareManifestRequest struct {
	Index    uint64
	TreeSize uint64
}

// InclusionProof contains the value at the requested index and the proof to the
// requested tree size.
type InclusionProof struct {
	Value     []byte
	LeafIndex uint64
	Proof     [][]byte
}
