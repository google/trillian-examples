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

package api

import "fmt"

// FirmwareMetadata represents a firmware image and related info.
type FirmwareMetadata struct {
	////// What's this firmware for? //////

	// TODO(al): consider a vendor/device/publisher model

	// DeviceID specifies the target device for this firmware.
	DeviceID string

	////// What's its identity? //////

	// FirmwareRevision specifies which version of firmware this is.
	// TODO(al): consider whether to use a string
	FirmwareRevision uint64

	// FirmwareImageSHA512 is the SHA512 hash over the firmware image as it will
	// be delievered.
	FirmwareImageSHA512 []byte

	// ExpectedFirmwareMeasurement represents the expected measured state of the
	// device once the firmware is installed.
	ExpectedFirmwareMeasurement []byte

	///// What's its provenance? //////

	// BuildTimestamp is the time at which this build was published in RFC3339 format.
	// e.g. "1985-04-12T23:20:50.52Z"
	BuildTimestamp string
}

// String returns a human-readable representation of the firmware metadata info.
func (m FirmwareMetadata) String() string {
	return fmt.Sprintf("%s/v%d built at %s with image hash 0x%x", m.DeviceID, m.FirmwareRevision, m.BuildTimestamp, m.FirmwareImageSHA512)
}
