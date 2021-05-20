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

// Package impl is a the implementation of a tool to put firmware metadata into the log.
package impl

import (
	"context"
	"crypto/sha512"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	dummy_common "github.com/google/trillian-examples/binary_transparency/firmware/devices/dummy/common"
	"github.com/google/trillian-examples/binary_transparency/firmware/devices/usbarmory"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/client"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/crypto"
	"golang.org/x/mod/sumdb/note"
)

// PublishOpts encapsulates parameters for the publish Main below.
type PublishOpts struct {
	LogURL         string
	LogSigVerifier note.Verifier
	DeviceID       string
	Revision       uint64
	BinaryPath     string
	Timestamp      string
	OutputPath     string
}

// Main is the entrypoint for the implementation of the publisher.
func Main(ctx context.Context, opts PublishOpts) error {
	logURL, err := url.Parse(opts.LogURL)
	if err != nil {
		return fmt.Errorf("LogURL is invalid: %w", err)
	}

	metadata, fw, err := createManifest(opts)
	if err != nil {
		return fmt.Errorf("failed to create manifest: %w", err)
	}

	glog.Infof("Measurement: %x", metadata.ExpectedFirmwareMeasurement)

	js, err := createStatementJSON(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal statement: %w", err)
	}

	c := &client.SubmitClient{
		ReadonlyClient: &client.ReadonlyClient{
			LogURL:         logURL,
			LogSigVerifier: opts.LogSigVerifier,
		},
	}

	initialCP, err := c.GetCheckpoint()
	if err != nil {
		return fmt.Errorf("failed to get a pre-submission checkpoint from log: %w", err)
	}

	glog.Info("Submitting entry...")
	if err := c.PublishFirmware(js, fw); err != nil {
		return fmt.Errorf("couldn't submit statement: %w", err)
	}

	glog.Info("Successfully submitted entry, waiting for inclusion...")
	cp, consistency, ip, err := client.AwaitInclusion(ctx, c.ReadonlyClient, *initialCP, js)
	if err != nil {
		glog.Errorf("Failed while waiting for inclusion: %v", err)
		glog.Warningf("Failed checkpoint: %s", cp)
		glog.Warningf("Failed consistency proof: %x", consistency)
		glog.Warningf("Failed inclusion proof: %x", ip)
		return fmt.Errorf("bailing: %w", err)
	}

	glog.Infof("Successfully logged %s", js)

	if len(opts.OutputPath) > 0 {
		glog.Infof("Creating update package file %q...", opts.OutputPath)
		pb, err := json.Marshal(
			api.ProofBundle{
				ManifestStatement: js,
				Checkpoint:        cp.Envelope,
				InclusionProof:    ip,
			})
		if err != nil {
			return fmt.Errorf("failed to marshal ProofBundle: %w", err)
		}

		bundle := api.UpdatePackage{
			FirmwareImage: fw,
			ProofBundle:   pb,
		}

		f, err := os.OpenFile(opts.OutputPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.ModePerm)
		if err != nil {
			return fmt.Errorf("failed to create output package file %q: %w", opts.OutputPath, err)
		}
		defer f.Close()

		if err := json.NewEncoder(f).Encode(bundle); err != nil {
			return fmt.Errorf("failed to encode output package JSON: %w", err)
		}
		glog.Infof("Successfully created update package file %q", opts.OutputPath)
	}
	return nil
}

func createManifest(opts PublishOpts) (api.FirmwareMetadata, []byte, error) {
	var measure func([]byte) ([]byte, error)
	switch opts.DeviceID {
	case "armory":
		measure = usbarmory.ExpectedMeasurement
	case "dummy":
		measure = dummy_common.ExpectedMeasurement
	default:
		return api.FirmwareMetadata{}, nil, errors.New("DeviceID must be one of: 'dummy', 'armory'")
	}

	fw, err := ioutil.ReadFile(opts.BinaryPath)
	if err != nil {
		return api.FirmwareMetadata{}, nil, fmt.Errorf("failed to read %q: %w", opts.BinaryPath, err)
	}

	h := sha512.Sum512(fw)

	m, err := measure(fw)
	if err != nil {
		return api.FirmwareMetadata{}, nil, fmt.Errorf("failed to calculate expected measurement for firmware: %w", err)
	}

	buildTime := opts.Timestamp
	if buildTime == "" {
		buildTime = time.Now().Format(time.RFC3339)
	}
	metadata := api.FirmwareMetadata{
		DeviceID:                    opts.DeviceID,
		FirmwareRevision:            opts.Revision,
		FirmwareImageSHA512:         h[:],
		ExpectedFirmwareMeasurement: m,
		BuildTimestamp:              buildTime,
	}

	return metadata, fw, nil
}

func createStatementJSON(m api.FirmwareMetadata) ([]byte, error) {
	js, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}
	sig, err := crypto.Publisher.SignMessage(api.FirmwareMetadataType, js)
	if err != nil {
		return nil, fmt.Errorf("failed to generate signature: %w", err)
	}

	statement := api.SignedStatement{
		Type:      api.FirmwareMetadataType,
		Statement: js,
		Signature: sig,
	}

	return json.Marshal(statement)
}
