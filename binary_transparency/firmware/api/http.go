package api

const (
	// HTTPAddFirmware is the path of the URL to publish a firmware entry.
	HTTPAddFirmware = "/ft/v0/add-firmware"
	// HTTPGetConsistency is the path of the URL to get a consistency proof between log roots.
	HTTPGetConsistency = "/ft/v0/get-consistency"
	// HTTPGetManifestEntries is the path of the URL to get firmware manifest entries with inclusion proofs.
	HTTPGetManifestEntries = "/ft/v0/get-firmware-manifest-entries"
	// HTTPGetRoot is the path of the URL to get a recent log root.
	HTTPGetRoot = "/ft/v0/get-root"
)
