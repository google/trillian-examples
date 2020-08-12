package api

// FirmwareMetadata represents a firmware image and related info.
type FirmwareMetadata struct {
	////// What's this firmware for? //////

	// TODO(al): consider a vendor/device/publisher model

	// DeviceID specifies the target device for this firmware.
	DeviceID string

	////// What's its identity? //////

	// FirmwareRevision specifies which version of firmware this is.
	// TODO(al): consider whether to use a string
	FirmwareRevision uint64

	// FirmwareImageSHA512 is the SHA512 hash over the firmware image as it will
	// be delievered.
	FirmwareImageSHA512 []byte

	// ExpectedFirmwareMeasurement represents the expected measured state of the
	// device once the firmware is installed.
	ExpectedFirmwareMeasurement []byte

	///// What's its provenance? //////

	// BuildTimestamp is the time at which this build was published in RFC3339 format.
	// e.g. "1985-04-12T23:20:50.52Z"
	BuildTimestamp string
}

// FirmwareStatement is the embodiment of the Statement in the claimant model for this demo.
type FirmwareStatement struct {
	// The serialised form of a FirmwareMetadata struct.
	// TODO(al): settle on serialisation format for this.
	Metadata []byte

	// Signature is the bytestream of the signature over the Raw field above.
	Signature []byte
}
