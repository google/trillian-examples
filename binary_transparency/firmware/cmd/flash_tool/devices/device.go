package devices

import (
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
)

// ErrNeedsInit error type indicates that the device is as yet uninitialised
// and the device may need to be "force" flashed.
type ErrNeedsInit = error

// Device represents an updatable device.
//
// Drivers for individual devices would be bound to this interface, which
// allows a generic flash tool to control the secure update process.
type Device interface {
	// DeviceCheckpoint returns the log checkpoint used during the last firmware update.
	DeviceCheckpoint() (api.LogCheckpoint, error)
	// ApplyUpdate applies the provided update to the device.
	ApplyUpdate(api.UpdatePackage) error
}
