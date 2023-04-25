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

// Package flash holds code to deal with the USB armory SD card storage.
package flash

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/cmd/flash_tool/devices"
)

const (
	// bundlePath is the filename within the proof partition where the armory
	// expects to fund the proof bundle.
	bundlePath = "bundle.json"
)

// Device represents the flash storage for the unsarmory.
type Device struct {
	fwDevPath  string
	bundlePath string

	// bundle holds all the update data except the firmware image.
	bundle api.ProofBundle
}

var _ devices.Device = Device{}

// New creates a new usbarmory device instance.
//
// storage is a comma separated string with the format:
// <proof_mount_point_path>,<firmware_block_device>
func New(storage string) (*Device, error) {
	bits := strings.Split(storage, ",")
	if len(bits) != 2 {
		return nil, errors.New("storage should be '<proof mount point path>,<firmware block device path>'")
	}
	proofDir, fwDevPath := bits[0], bits[1]

	dStat, err := os.Stat(proofDir)
	if err != nil {
		return nil, fmt.Errorf("unable to stat %q: %w", proofDir, err)
	}
	if !dStat.Mode().IsDir() {
		return nil, fmt.Errorf("%q is not a directory", proofDir)
	}

	d := &Device{
		fwDevPath:  filepath.Clean(fwDevPath),
		bundlePath: filepath.Clean(filepath.Join(proofDir, bundlePath)),
	}

	f, err := os.OpenFile(d.bundlePath, os.O_RDONLY, os.ModePerm)
	if err != nil {
		if os.IsNotExist(err) {
			return d, devices.ErrNeedsInit(fmt.Errorf("couldn't read bundle file %q: %w", d.bundlePath, err))
		}
		return d, fmt.Errorf("failed to read bundle file %q: %w", d.bundlePath, err)
	}
	defer func() {
		_ = f.Close()
	}()

	err = json.NewDecoder(f).Decode(&d.bundle)
	return d, err
}

// DeviceCheckpoint returns the latest log checkpoint stored on the device.
func (d Device) DeviceCheckpoint() ([]byte, error) {
	return d.bundle.Checkpoint, nil
}

// ApplyUpdate applies the firmware update to the armory SD Card device.
// The firmware image is written directly to the unikernel partition of the device
// (the raw block device is specified by the --armory_unikernel_dev flag),
// and proof bundle is stored in the proof partition of the device which must be
// mounted at the location specified by the --armory_proof_mount_point flag).
//
// TODO(al): see what can be done to make this easier to use.
func (d Device) ApplyUpdate(u api.UpdatePackage) error {
	if err := os.WriteFile(d.bundlePath, u.ProofBundle, os.ModePerm); err != nil {
		return fmt.Errorf("failed to write proof bundle to %q: %w", d.bundlePath, err)
	}

	fw := u.FirmwareImage
	if err := os.WriteFile(d.fwDevPath, fw, os.ModePerm); err != nil {
		return fmt.Errorf("failed to write firmware image to %q: %w", d.fwDevPath, err)
	}

	return nil
}
