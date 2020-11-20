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

package dummy

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/cmd/flash_tool/devices"
)

var (
	dummyDirectory = flag.String("dummy_storage_dir", "/tmp/dummy_device", "Directory path to store the dummy device's state")
)

const (
	bundlePath   = "bundle.json"
	firmwarePath = "firmware.bin"
)

// Device is a fake device using the local filesystem for storage.
type Device struct {
	// bundle holds all the update data except the firmware image.
	bundle api.ProofBundle
}

var _ devices.Device = Device{}

// NewFromFlags creates a new dummy device instance using data from flags.
// TODO(al): figure out how/whether to remove the flag from in here.
func NewFromFlags() (*Device, error) {
	dStat, err := os.Stat(*dummyDirectory)
	if err != nil {
		return nil, fmt.Errorf("unable to stat dummy_storage_dir %q: %w", *dummyDirectory, err)
	}
	if !dStat.Mode().IsDir() {
		return nil, fmt.Errorf("dummy_storage_dir %q is not a directory", *dummyDirectory)
	}

	d := &Device{}

	fPath := filepath.Join(*dummyDirectory, bundlePath)
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
func (d Device) DeviceCheckpoint() (api.LogCheckpoint, error) {
	return d.bundle.Checkpoint, nil
}

// ApplyUpdate applies the firmware update to the dummy device.
// The firmware image is stored in the dummy state directory in the firmware.bin file,
// and the rest of the update bundle is stored in the bundle.json file.
func (d Device) ApplyUpdate(u api.UpdatePackage) error {
	fwFile := filepath.Join(*dummyDirectory, firmwarePath)
	bundleFile := filepath.Join(*dummyDirectory, bundlePath)

	proof, err := json.Marshal(u.ProofBundle)
	if err != nil {
		return fmt.Errorf("failed to marshal proof bundle: %q", err)
	}
	if err := ioutil.WriteFile(bundleFile, proof, os.ModePerm); err != nil {
		return fmt.Errorf("failed to write proof bundle to %q: %q", bundleFile, err)
	}

	fw := u.FirmwareImage
	if err := ioutil.WriteFile(fwFile, fw, os.ModePerm); err != nil {
		return fmt.Errorf("failed to write firmware image to %q: %q", fwFile, err)
	}

	return nil
}
