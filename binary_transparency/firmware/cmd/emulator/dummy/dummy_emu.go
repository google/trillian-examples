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

// dummy_emu is an "emulator" for the dummy device.
//
// This emulator shows how the firmware transparency artifacts stored
// when firmware is updates are validated on boot to protect the device against
// tampering.
//
// The device's initialisation routines in "ROM" are where this validation
// takes place, and the device only "jumps into" the firmware payload once
// successful validation has taken place.
//
// Usage:
//   go run ./cmd/emulator/dummy --logtostderr --dummy_storage_dir=/tmp/dummy_device
package main

import (
	"flag"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/devices/dummy/rom"
)

func main() {
	flag.Parse()

	boot, err := rom.ResetFromFlags()
	if err != nil {
		glog.Exitf("ROM: %q", err)
	}

	if err := boot(); err != nil {
		glog.Warningf("boot(): %q", err)
	}
}
