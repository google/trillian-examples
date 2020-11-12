package api

// UpdatePackage represents an "OTA" update bundle.
type UpdatePackage struct {
	// FirmwareImage is the actual firmware image itself.
	FirmwareImage []byte
	// ProofBundle holds the various artifacts required to validate the firmware image.
	ProofBundle ProofBundle
}

// ProofBundle contains the manifest and associated proofs for a given firmware image.
type ProofBundle struct {
	// ManifestStatement is the json representation of an `api.FirmwareMetadata` struct.
	ManifestStatement []byte
	// Checkpoint must represent a tree which includes the ManifestStatement.
	Checkpoint LogCheckpoint
	// InclusionProof is a proof to Checkpoint for ManifestStatement.
	InclusionProof InclusionProof
}
