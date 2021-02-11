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

// Package impl is the implementation of a hacker tool for modifying proof bundles.
//
// It can update the expected measurement and firmware image hashes of the specified
// proof bundle file, optionally sign the firmware statement, and finally write the bundle
// to the specified output file.
package impl

import (
	"crypto/sha512"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	dummy_common "github.com/google/trillian-examples/binary_transparency/firmware/devices/dummy/common"
	"github.com/google/trillian-examples/binary_transparency/firmware/devices/usbarmory"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/crypto"
)

// ModifyBundleOpts encapsulates parameters for the modify bundle Main below.
type ModifyBundleOpts struct {
	BinaryPath string
	DeviceID   string
	Input      string
	Output     string
	Sign       bool
}

// Main is the modify bundle entrypoint.
func Main(opts ModifyBundleOpts) error {
	if len(opts.BinaryPath) == 0 {
		return errors.New("must specify BinaryPath")
	}

	bundleRaw, err := ioutil.ReadFile(opts.Input)
	if err != nil {
		return fmt.Errorf("failed to read transparency bundle %q: %w", opts.Input, err)
	}
	var pb api.ProofBundle
	if err := json.Unmarshal(bundleRaw, &pb); err != nil {
		return fmt.Errorf("failed to parse proof bundle file: %w", err)
	}
	var fs api.SignedStatement
	if err := json.Unmarshal(pb.ManifestStatement, &fs); err != nil {
		return fmt.Errorf("failed to parse SignedStatement: %w", err)
	}
	var fm api.FirmwareMetadata
	if err := json.Unmarshal(fs.Statement, &fm); err != nil {
		return fmt.Errorf("failed to parse FirmwareMetadata: %w", err)
	}

	var measure func([]byte) ([]byte, error)
	switch opts.DeviceID {
	case "armory":
		measure = usbarmory.ExpectedMeasurement
	case "dummy":
		measure = dummy_common.ExpectedMeasurement
	default:
		return fmt.Errorf("DeviceID must be one of: 'dummy', 'armory'")
	}

	fw, err := ioutil.ReadFile(opts.BinaryPath)
	if err != nil {
		return fmt.Errorf("failed to read %q: %w", opts.BinaryPath, err)
	}

	h := sha512.Sum512(fw)

	m, err := measure(fw)
	if err != nil {
		return fmt.Errorf("failed to calculate expected measurement for firmware: %w", err)
	}

	fm.ExpectedFirmwareMeasurement = m
	fm.FirmwareImageSHA512 = h[:]

	rawMeta, err := json.Marshal(fm)
	if err != nil {
		return fmt.Errorf("failed to marshal FirmwareMetadata: %w", err)
	}
	fs.Statement = rawMeta

	if opts.Sign {
		// Use stolen key to re-sign the dodgy metadata
		sig, err := crypto.Publisher.SignMessage(api.FirmwareMetadataType, rawMeta)
		if err != nil {
			return fmt.Errorf("failed to sign FirmwareMetadata: %w", err)
		}
		fs.Signature = sig
	}

	rawFS, err := json.Marshal(fs)
	if err != nil {
		return fmt.Errorf("failed to marshal SignedStatement: %w", err)
	}

	pb.ManifestStatement = rawFS

	rawPB, err := json.Marshal(pb)
	if err != nil {
		return fmt.Errorf("failed to marshal ProofBundle: %w", err)
	}

	if err := ioutil.WriteFile(opts.Output, rawPB, 0x644); err != nil {
		return fmt.Errorf("failed to write modified bundle to %q: %w", opts.Output, err)
	}

	return nil
}
