// Copyright 2020 Google LLC. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
