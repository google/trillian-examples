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

// Package impl is the implementation of the emulator for the dummy device.
package impl

import (
	"fmt"

	"github.com/google/trillian-examples/binary_transparency/firmware/devices/dummy/rom"
)

// EmulatorOpts encapsulates the parameters for running the emulator.
type EmulatorOpts struct {
	DeviceStorage string
}

// Main is the entry point for the dummy emulator
func Main(opts EmulatorOpts) error {
	boot, err := rom.Reset(opts.DeviceStorage)
	if err != nil {
		return fmt.Errorf("ROM: %w", err)
	}

	if err := boot(); err != nil {
		return fmt.Errorf("boot(): %w", err)
	}

	return nil
}
