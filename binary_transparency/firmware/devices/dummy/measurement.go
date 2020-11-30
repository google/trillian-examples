package dummy

import (
	"crypto/sha512"
	"fmt"
)

// ExpectedMeasurement returns the expected on-device measurement hash for the
// given firmware image.
//
// For the dummy device, this is SHA512("dummy"||img)
func ExpectedMeasurement(img []byte) ([]byte, error) {
	hasher := sha512.New()
	if _, err := hasher.Write([]byte("dummy")); err != nil {
		return nil, fmt.Errorf("failed to write measurement domain prefix to hasher: %w", err)
	}
	if _, err := hasher.Write(img); err != nil {
		return nil, fmt.Errorf("failed to write firmware image to hasher: %w", err)
	}
	fwHash := hasher.Sum(nil)
	return fwHash[:], nil
}
