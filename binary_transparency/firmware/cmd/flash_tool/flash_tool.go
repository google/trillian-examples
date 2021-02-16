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
	"flag"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/cmd/flash_tool/impl"
)

var (
	deviceID      = flag.String("device", "", "One of [dummy, armory]")
	logURL        = flag.String("log_url", "http://localhost:8000", "Base URL of the log HTTP API")
	witnessURL    = flag.String("witness_url", "", "URL to fetch the Witness")
	updateFile    = flag.String("update_file", "", "File path to read the update package from")
	force         = flag.Bool("force", false, "Ignore errors and force update")
	deviceStorage = flag.String("device_storage", "", "Storage description string for selected device")
)

func main() {
	flag.Parse()

	if err := impl.Main(impl.FlashOpts{
		DeviceID:      *deviceID,
		LogURL:        *logURL,
		WitnessURL:    *witnessURL,
		UpdateFile:    *updateFile,
		Force:         *force,
		DeviceStorage: *deviceStorage,
	}); err != nil {
		glog.Exit(err.Error())
	}
}
