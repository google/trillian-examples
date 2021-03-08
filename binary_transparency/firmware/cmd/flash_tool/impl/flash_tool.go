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

// package impl is the implementation of a util to flash firmware update packages created by the publisher tool onto devices.
//
// Currently, the only device is a dummy device, which simply sorts the firmware+metadata on local disk.
package impl

import (
	"crypto/sha512"
	"encoding/json"
	"errors"
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

// FlashOpts encapsulates flash tool parameters.
type FlashOpts struct {
	DeviceID      string
	LogURL        string
	WitnessURL    string
	UpdateFile    string
	Force         bool
	DeviceStorage string
}

func Main(opts FlashOpts) error {
	logURL, err := url.Parse(opts.LogURL)
	if err != nil {
		return fmt.Errorf("log_url is invalid: %w", err)
	}
	c := &client.ReadonlyClient{LogURL: logURL}

	up, err := readUpdateFile(opts.UpdateFile)
	if err != nil {
		return fmt.Errorf("Failed to read update package file: %w", err)
	}
	// TODO(al): check signature on checkpoints when they're added.

	var dev devices.Device
	switch opts.DeviceID {
	case "armory":
		dev, err = armory_flash.New(opts.DeviceStorage)
	case "dummy":
		dev, err = dummy.New(opts.DeviceStorage)
	default:
		return errors.New("device must be one of: 'dummy', 'armory'")
	}
	if err != nil {
		switch t := err.(type) {
		case devices.ErrNeedsInit:
			err := fmt.Errorf("device needs to be force initialised: %w", err)
			if !opts.Force {
				return err
			}
			glog.Warning(err)
		default:
			err := fmt.Errorf("failed to open device: %w", t)
			if !opts.Force {
				return err
			}
			glog.Warning(err)
		}
	}
	pb, err := verifyUpdate(c, up, dev)
	if err != nil {
		err := fmt.Errorf("failed to validate update: %w", err)
		if !opts.Force {
			return err
		}
		glog.Warning(err)
	}

	if len(opts.WitnessURL) > 0 {
		wURL, err := url.Parse(opts.WitnessURL)
		if err != nil {
			return fmt.Errorf("witness_url is invalid: %w", err)
		}
		wc := client.WitnessClient{URL: wURL}

		if err := verifyWitness(c, pb, wc); err != nil {
			err := fmt.Errorf("failed to verify update with witness: %w", err)
			if !opts.Force {
				return err
			}
			glog.Warning(err)
		}
	}

	// TODO(mhutchinson): query the map:
	// 1. Check that the map root commits to a log root consistent with the checkpoints above
	// 2. Look up the aggregation under the key: fmt.Sprintf("summary:%d", pb.InclusionProof.LeafIndex)

	glog.Info("Update verified, about to apply to device...")

	if err := dev.ApplyUpdate(up); err != nil {
		return fmt.Errorf("failed to apply update to device: %w", err)
	}

	glog.Info("Update applied.")
	return nil
}

func readUpdateFile(path string) (api.UpdatePackage, error) {
	if len(path) == 0 {
		return api.UpdatePackage{}, errors.New("must specify update_file")
	}

	f, err := os.OpenFile(path, os.O_RDONLY, os.ModePerm)
	if err != nil {
		glog.Exitf("Failed to open update package file %q: %q", path, err)
	}
	defer f.Close()

	var up api.UpdatePackage
	if err := json.NewDecoder(f).Decode(&up); err != nil {
		glog.Exitf("Failed to parse update package file: %q", err)
	}
	return up, nil
}

// getConsistencyFunc executes on a given client context and returns a
// consistency function.
func getConsistencyFunc(c *client.ReadonlyClient) func(from, to uint64) ([][]byte, error) {
	cpFunc := func(from, to uint64) ([][]byte, error) {
		var cp [][]byte
		if from > 0 {
			r, err := c.GetConsistencyProof(api.GetConsistencyRequest{From: from, To: to})
			if err != nil {
				return nil, fmt.Errorf("failed to fetch consistency proof: %w", err)
			}
			cp = r.Proof
		}
		return cp, nil
	}
	return cpFunc
}

// verifyUpdate checks that an update package is self-consistent and returns a verified proof bundle
func verifyUpdate(c *client.ReadonlyClient, up api.UpdatePackage, dev devices.Device) (api.ProofBundle, error) {
	var pb api.ProofBundle

	// Get the consistency proof for the bundle
	dc, err := dev.DeviceCheckpoint()
	if err != nil {
		return pb, fmt.Errorf("failed to fetch the device checkpoint: %w", err)
	}

	cpFunc := getConsistencyFunc(c)
	fwHash := sha512.Sum512(up.FirmwareImage)
	pb, err = verify.BundleForUpdate(up.ProofBundle, fwHash[:], dc, cpFunc)
	if err != nil {
		return pb, fmt.Errorf("failed to verify proof bundle: %w", err)
	}
	return pb, nil
}

// verifyWitness checks that an update package is consistent with witness
func verifyWitness(c *client.ReadonlyClient, pb api.ProofBundle, wc client.WitnessClient) error {
	wcp, err := wc.GetWitnessCheckpoint()
	if err != nil {
		return fmt.Errorf("failed to fetch the witness checkpoint: %w", err)
	}

	if wcp.TreeSize == 0 {
		return fmt.Errorf("No witness checkpoint to verify")
	}
	cpFunc := getConsistencyFunc(c)
	if err := verify.BundleValidateWitness(pb, (*wcp), cpFunc); err != nil {
		return fmt.Errorf("failed to verify proof bundle: %w", err)
	}
	return nil
}
