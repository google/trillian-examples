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

// Package dummy provides a fake device to demo flashing firmware.
package dummy

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/cmd/flash_tool/devices"
)

const (
	bundlePath   = "bundle.json"
	firmwarePath = "firmware.bin"
)

// Device is a fake device using the local filesystem for storage.
type Device struct {
	// bundle holds all the update data except the firmware image.
	bundle api.ProofBundle

	storage string
}

var _ devices.Device = Device{}

// New creates a new dummy device instance using data from flags.
// TODO(al): figure out how/whether to remove the flag from in here.
func New(storage string) (*Device, error) {
	dStat, err := os.Stat(storage)
	if err != nil {
		return nil, fmt.Errorf("unable to stat device storage dir %q: %w", storage, err)
	}
	if !dStat.Mode().IsDir() {
		return nil, fmt.Errorf("device storage %q is not a directory", storage)
	}

	d := &Device{
		storage: storage,
	}

	fPath := filepath.Join(storage, bundlePath)
	f, err := os.OpenFile(fPath, os.O_RDONLY, os.ModePerm)
	if err != nil {
		if os.IsNotExist(err) {
			return d, devices.ErrNeedsInit(fmt.Errorf("couldn't read bundle file %q: %w", fPath, err))
		}
		return d, fmt.Errorf("failed to read bundle file %q: %w", fPath, err)
	}
	defer f.Close()

	err = json.NewDecoder(f).Decode(&d.bundle)
	return d, err
}

// DeviceCheckpoint returns the latest log checkpoint stored on the device.
func (d Device) DeviceCheckpoint() ([]byte, error) {
	return d.bundle.Checkpoint, nil
}

// ApplyUpdate applies the firmware update to the dummy device.
// The firmware image is stored in the dummy state directory in the firmware.bin file,
// and the rest of the update bundle is stored in the bundle.json file.
func (d Device) ApplyUpdate(u api.UpdatePackage) error {
	fwFile := filepath.Join(d.storage, firmwarePath)
	bundleFile := filepath.Join(d.storage, bundlePath)

	if err := ioutil.WriteFile(bundleFile, u.ProofBundle, os.ModePerm); err != nil {
		return fmt.Errorf("failed to write proof bundle to %q: %q", bundleFile, err)
	}

	fw := u.FirmwareImage
	if err := ioutil.WriteFile(fwFile, fw, os.ModePerm); err != nil {
		return fmt.Errorf("failed to write firmware image to %q: %q", fwFile, err)
	}

	return nil
}
