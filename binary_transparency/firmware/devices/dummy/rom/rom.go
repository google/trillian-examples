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

package rom

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/devices/dummy/common"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/crypto"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/verify"
)

var (
	dummyDirectory = flag.String("dummy_storage_dir", "/tmp/dummy_device", "Directory path of the dummy device's state storage")
)

const (
	bundlePath   = "bundle.json"
	firmwarePath = "firmware.bin"
)

// Chain represents the next stage in the boot process.
type Chain func() error

// ResetFromFlags is intended to emulate the early stage boot process of a device.
//
// It's separate from the device emulator code to highlight that the process of
// verifying the local firmware/proofs/etc. could be done on-device at
// as part of the boot-ROM execution to provide some tamper-resistant properties
// to the firmware installed on the device.
//
// Other, real-world, devices with secure elements may be able to optimise this
// process by checking once and leveraging properties of the hardware.
//
// Returns the first link in the boot chain as a func.
func ResetFromFlags() (Chain, error) {
	glog.Info("----RESET----")
	glog.Info("Powering up bananas, configuring Romulans, feeding the watchdogs")

	glog.Infof("Configuring flash and loading FT artifacts from %q...", *dummyDirectory)

	fwFile := filepath.Clean(filepath.Join(*dummyDirectory, firmwarePath))
	bundleFile := filepath.Clean(filepath.Join(*dummyDirectory, bundlePath))

	b, err := readAndParseBundle(bundleFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read transparency bundle: %w", err)
	}

	fw, err := ioutil.ReadFile(fwFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read firmware: %w", err)
	}

	fwMeasurement, err := common.ExpectedMeasurement(fw)
	if err != nil {
		return nil, fmt.Errorf("failed calculate measurement: %w", err)
	}

	// validate bundle
	if err := verify.BundleForBoot(b, fwMeasurement[:]); err != nil {
		return nil, fmt.Errorf("failed to verify bundle: %w", err)
	}

	var stmt api.FirmwareStatement
	if err := json.Unmarshal(b.ManifestStatement, &stmt); err != nil {
		return nil, fmt.Errorf("error parsing firmware metadata: %w", err)
	}
	// TODO(al): check signature on checkpoints when they're added.
	// verify the signature
	if err := crypto.VerifySignature(stmt.Metadata, stmt.Signature); err != nil {
		return nil, fmt.Errorf("failed to verify signature: %w", err)
	}
	// boot
	glog.Infof("Prepared to boot %s", stmt.Metadata)
	boot1 := func() error {
		return bootWasm("main", fw)
	}
	return boot1, nil
}

func readAndParseBundle(bundleFile string) (api.ProofBundle, error) {
	bundleRaw, err := ioutil.ReadFile(bundleFile)
	if err != nil {
		return api.ProofBundle{}, fmt.Errorf("failed to read transparency bundle: %w", err)
	}
	var pb api.ProofBundle
	if err := json.Unmarshal(bundleRaw, &pb); err != nil {
		return api.ProofBundle{}, fmt.Errorf("failed to parse proof bundle file: %w", err)
	}
	return pb, nil
}
