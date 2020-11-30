package usbarmory

import (
	"crypto/sha256"
	"fmt"
)

const measurementDomainPrefix = "armory_mkii"

// ExpectedMeasurement returns the expected on-device measurement hash for the
// given firmware image.
//
// For the USB Armorye, this is SHA256("armory_mkii"||img)
func ExpectedMeasurement(img []byte) ([]byte, error) {
	hasher := sha256.New()
	if _, err := hasher.Write([]byte(measurementDomainPrefix)); err != nil {
		return nil, fmt.Errorf("failed to write measurement domain prefix to hasher: %w", err)
	}
	if _, err := hasher.Write(img); err != nil {
		return nil, fmt.Errorf("failed to write firmware image to hasher: %w", err)
	}
	fwHash := hasher.Sum(nil)
	return fwHash[:], nil
}
