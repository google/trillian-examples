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

// modify_bundle is a hacker tool for modifying proof bundles.
//
// It can update the expected measurement and firmware image hashes of the specified
// proof bundle file, optionally sign the firmware statement, and finally write the bundle
// to the specified output file.
//
// Usage:
//   go run ./cmd/hacker/modify_bundle/ \
//      --logtostderr \
//      --device=[dummy,usbarmory] \
//      --binary=/path/to/new/firmware/image \
//      --input=/path/to/bundle.json \
//      --output=/path/to/bundle.json
package main

import (
	"crypto/sha512"
	"encoding/json"
	"flag"
	"io/ioutil"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	dummy_common "github.com/google/trillian-examples/binary_transparency/firmware/devices/dummy/common"
	"github.com/google/trillian-examples/binary_transparency/firmware/devices/usbarmory"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/crypto"
)

var (
	input      = flag.String("input", "", "File path read input ProofBundle from.")
	output     = flag.String("output", "", "File path to write output ProofBundle to.")
	sign       = flag.Bool("sign", true, "Whether to use stolen key to sign manifest.")
	binaryPath = flag.String("binary", "", "Replacement binary image.")
	deviceID   = flag.String("device", "", "One of [dummy,usbarmory].")
)

func main() {
	flag.Parse()

	if len(*binaryPath) == 0 {
		glog.Exit("Must specify --binary flag.")
	}

	bundleRaw, err := ioutil.ReadFile(*input)
	if err != nil {
		glog.Exitf("Failed to read transparency bundle %q: %q", *input, err)
	}
	var pb api.ProofBundle
	if err := json.Unmarshal(bundleRaw, &pb); err != nil {
		glog.Exitf("Failed to parse proof bundle file: %q", err)
	}
	var fs api.FirmwareStatement
	if err := json.Unmarshal(pb.ManifestStatement, &fs); err != nil {
		glog.Exitf("Failed to parse FirmwareStatement: %q", err)
	}
	var fm api.FirmwareMetadata
	if err := json.Unmarshal(fs.Metadata, &fm); err != nil {
		glog.Exitf("Failed to parse FirmwareMetadata: %q", err)
	}

	var measure func([]byte) ([]byte, error)
	switch *deviceID {
	case "armory":
		measure = usbarmory.ExpectedMeasurement
	case "dummy":
		measure = dummy_common.ExpectedMeasurement
	default:
		glog.Exit("--device must be one of: 'dummy', 'armory'")
	}

	fw, err := ioutil.ReadFile(*binaryPath)
	if err != nil {
		glog.Exitf("Failed to read %q: %q", *binaryPath, err)
	}

	h := sha512.Sum512(fw)

	m, err := measure(fw)
	if err != nil {
		glog.Exitf("Failed to calculate expected measurement for firmware: %q", err)
	}

	fm.ExpectedFirmwareMeasurement = m
	fm.FirmwareImageSHA512 = h[:]

	rawMeta, err := json.Marshal(fm)
	if err != nil {
		glog.Exitf("Failed to marshal FirmwareMetadata: %q", err)
	}
	fs.Metadata = rawMeta

	if *sign {
		// Use stolen key to re-sign the dodgy metadata
		sig, err := crypto.SignMessage(rawMeta)
		if err != nil {
			glog.Exitf("Failed to sign FirmwareMetadata: %q", err)
		}
		fs.Signature = sig
	}

	rawFS, err := json.Marshal(fs)
	if err != nil {
		glog.Exitf("Failed to marshal FirmwareStatement: %q", err)
	}

	pb.ManifestStatement = rawFS

	rawPB, err := json.Marshal(pb)
	if err != nil {
		glog.Exitf("Failed to marshal ProofBundle: %q", err)
	}

	if err := ioutil.WriteFile(*output, rawPB, 0x644); err != nil {
		glog.Exitf("Failed to write modified bundle to %q: %q", *output, err)
	}
}
