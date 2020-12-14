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

// flash_tool is a util to flash firmware update packages created by the publisher tool onto devices.
//
// Currently, the only device is a dummy device, which simply sorts the firmware+metadata on local disk.
//
// Usage:
//   go run ./cmd/flash_tool/ --logtostderr --dummy_storage_dir=/path/to/dir --update_file=/path/to/update.json
//
// The first time you use this tool there will be no prior firmware metadata
// stored on the device and the tool will fail.  In this case, use the --force
// flag to apply the update anyway thereby creating the metadata.
// Subsequent invocations should then work without needing the --force flag.
package main

import (
	"crypto/sha512"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/cmd/flash_tool/devices"
	"github.com/google/trillian-examples/binary_transparency/firmware/devices/dummy"
	armory_flash "github.com/google/trillian-examples/binary_transparency/firmware/devices/usbarmory/flash"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/client"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/verify"
)

var (
	deviceID   = flag.String("device", "", "One of [dummy, armory]")
	logURL     = flag.String("log_url", "http://localhost:8000", "Base URL of the log HTTP API")
	updateFile = flag.String("update_file", "", "File path to read the update package from")
	force      = flag.Bool("force", false, "Ignore errors and force update")
)

func fatal(msg string) {
	if !*force {
		glog.Exit(msg)
	}
	glog.Warning(msg)
}

func main() {
	flag.Parse()

	logURL, err := url.Parse(*logURL)
	if err != nil {
		glog.Exitf("log_url is invalid: %v", err)
	}
	c := &client.ReadonlyClient{LogURL: logURL}

	up, err := readUpdateFileFromFlags()
	if err != nil {
		glog.Exitf("Failed to read update package file: %q", err)
	}
	// TODO(al): check signature on checkpoints when they're added.

	var dev devices.Device
	switch *deviceID {
	case "armory":
		dev, err = armory_flash.NewFromFlags()
	case "dummy":
		dev, err = dummy.NewFromFlags()
	default:
		glog.Exit("--device must be one of: 'dummy', 'armory'")
	}
	if err != nil {
		switch t := err.(type) {
		case devices.ErrNeedsInit:
			fatal(fmt.Sprintf("Device needs to be force initialised: %q", err))
		default:
			fatal(fmt.Sprintf("Failed to open device: %q", t))
		}
	}

	if err := verifyUpdate(c, up, dev); err != nil {
		fatal(fmt.Sprintf("Failed to validate update: %q", err))
	}

	glog.Info("Update verified, about to apply to device...")

	if err := dev.ApplyUpdate(up); err != nil {
		glog.Exitf("Failed to apply update to device: %q", err)
	}

	glog.Info("Update applied.")

}

func readUpdateFileFromFlags() (api.UpdatePackage, error) {
	if len(*updateFile) == 0 {
		return api.UpdatePackage{}, errors.New("must specify update_file")
	}

	f, err := os.OpenFile(*updateFile, os.O_RDONLY, os.ModePerm)
	if err != nil {
		glog.Exitf("Failed to open update package file %q: %q", *updateFile, err)
	}
	defer f.Close()

	var up api.UpdatePackage
	if err := json.NewDecoder(f).Decode(&up); err != nil {
		glog.Exitf("Failed to parse update package file: %q", err)
	}
	return up, nil
}

// verifyUpdate checks that an update package is self-consistent.
func verifyUpdate(c *client.ReadonlyClient, up api.UpdatePackage, dev devices.Device) error {
	// Get the consistency proof for the bundle
	dc, err := dev.DeviceCheckpoint()
	if err != nil {
		return fmt.Errorf("failed to fetch the device checkpoint: %w", err)
	}

	cpFunc := func(from, to uint64) ([][]byte, error) {
		var cp [][]byte
		if dc.TreeSize > 0 {
			r, err := c.GetConsistencyProof(api.GetConsistencyRequest{From: from, To: to})
			if err != nil {
				return nil, fmt.Errorf("failed to fetch consistency proof: %w", err)
			}
			cp = r.Proof
		}
		return cp, nil
	}

	fwHash := sha512.Sum512(up.FirmwareImage)
	if err := verify.BundleForUpdate(up.ProofBundle, fwHash[:], dc, cpFunc); err != nil {
		return fmt.Errorf("failed to verify proof bundle: %w", err)
	}
	return nil
}
