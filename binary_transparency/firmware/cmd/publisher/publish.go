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
	"flag"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/cmd/publisher/impl"
)

var (
	logURL = flag.String("log_url", "http://localhost:8000", "Base URL of the log HTTP API")

	deviceID   = flag.String("device", "", "the target device for the firmware")
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

	if err := impl.Main(ctx, impl.PublishOpts{
		LogURL:     *logURL,
		DeviceID:   *deviceID,
		Revision:   *revision,
		BinaryPath: *binaryPath,
		Timestamp:  *timestamp,
		OutputPath: *outputPath,
	}); err != nil {
		glog.Exitf(err.Error())
	}
}
