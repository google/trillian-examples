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

// publish is a demo tool to put firmware metadata into the log.
package main

import (
	"context"
	"crypto/sha512"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/devices/dummy"
	"github.com/google/trillian-examples/binary_transparency/firmware/devices/usbarmory"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/client"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/crypto"
)

var (
	logURL = flag.String("log_url", "http://localhost:8000", "Base URL of the log HTTP API")

	deviceID   = flag.String("device", "dummy", "the target device for the firmware")
	revision   = flag.Uint64("revision", 1, "the version of the firmware")
	binaryPath = flag.String("binary_path", "", "file path to the firmware binary")
	timestamp  = flag.String("timestamp", "", "timestamp formatted as RFC3339, or empty to use current time")
	timeout    = flag.Duration("timeout", 5*time.Minute, "Duration to wait for inclusion of submitted metadata")
	outputPath = flag.String("output_path", "/tmp/update.ota", "File path to write the update package file to. This file is intended to be consumed by the flash_tool only.")
)

func main() {
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	logURL, err := url.Parse(*logURL)
	if err != nil {
		glog.Exitf("logURL is invalid: %v", err)
	}

	metadata, fw, err := createManifestFromFlags()
	if err != nil {
		glog.Exitf("Failed to create manifest: %v", err)
	}

	js, err := createStatementJSON(metadata)
	if err != nil {
		glog.Exitf("Failed to marshal statement: %v", err)
	}

	c := &client.SubmitClient{
		ReadonlyClient: &client.ReadonlyClient{
			LogURL: logURL,
		},
	}

	initialCP, err := c.GetCheckpoint()
	if err != nil {
		glog.Exitf("Failed to get a pre-submission checkpoint from log: %q", err)
	}

	glog.Info("Submitting entry...")
	if err := c.PublishFirmware(js, fw); err != nil {
		glog.Exitf("Couldn't submit statement: %v", err)
	}

	glog.Info("Successfully submitted entry, waiting for inclusion...")
	cp, consistency, ip, err := client.AwaitInclusion(ctx, c.ReadonlyClient, *initialCP, js)
	if err != nil {
		glog.Errorf("Failed while waiting for inclusion: %v", err)
		glog.Warningf("Failed checkpoint: %s", cp)
		glog.Warningf("Failed consistency proof: %x", consistency)
		glog.Warningf("Failed inclusion proof: %x", ip)
		glog.Exit("Bailing.")
	}

	glog.Infof("Successfully logged %s", js)

	if len(*outputPath) > 0 {
		glog.Infof("Creating update package file %q...", *outputPath)

		bundle := api.UpdatePackage{
			FirmwareImage: fw,
			ProofBundle: api.ProofBundle{
				ManifestStatement: js,
				Checkpoint:        cp,
				InclusionProof:    ip,
			},
		}

		f, err := os.OpenFile(*outputPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.ModePerm)
		if err != nil {
			glog.Exitf("Failed to create output package file %q: %q", *outputPath, err)
		}
		defer f.Close()

		if err := json.NewEncoder(f).Encode(bundle); err != nil {
			glog.Exitf("Failed to encode output package JSON: %q", err)
		}
		glog.Infof("Successfully created update package file %q", *outputPath)
	}
}

func createManifestFromFlags() (api.FirmwareMetadata, []byte, error) {
	var measure func([]byte) ([]byte, error)
	switch *deviceID {
	case "armory":
		measure = usbarmory.ExpectedMeasurement
	case "dummy":
		measure = dummy.ExpectedMeasurement
	default:
		glog.Exitf("--device must be one of: 'dummy', 'armory'")
	}

	fw, err := ioutil.ReadFile(*binaryPath)
	if err != nil {
		return api.FirmwareMetadata{}, nil, fmt.Errorf("failed to read %q: %w", *binaryPath, err)
	}

	h := sha512.Sum512(fw)
	m, err := measure(fw)
	if err != nil {
		return api.FirmwareMetadata{}, nil, fmt.Errorf("failed to calculate expected measurement for firmware: %w", err)
	}

	buildTime := *timestamp
	if buildTime == "" {
		buildTime = time.Now().Format(time.RFC3339)
	}
	metadata := api.FirmwareMetadata{
		DeviceID:                    *deviceID,
		FirmwareRevision:            *revision,
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
	sig, err := crypto.SignMessage(js)
	if err != nil {
		return nil, fmt.Errorf("failed to generate signature: %w", err)
	}

	statement := api.FirmwareStatement{
		Metadata:  js,
		Signature: sig,
	}

	return json.Marshal(statement)
}
