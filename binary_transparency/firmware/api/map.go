// Copyright 2021 Google LLC. All Rights Reserved.
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

// AggregatedFirmware represents the results of aggregating a single piece of firmware
// according to the rules described in #Aggregate().
type AggregatedFirmware struct {
	Index    uint64
	Firmware *FirmwareMetadata
	Good     bool
}

// DeviceReleaseLog represents firmware releases found for a single device ID.
// Entries are ordered by their sequence in the original log.
type DeviceReleaseLog struct {
	DeviceID  string
	Revisions []uint64
}
