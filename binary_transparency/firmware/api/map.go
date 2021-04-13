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

import "fmt"

const (
	// MapHTTPGetCheckpoint is the path of the URL to get a recent map checkpoint.
	MapHTTPGetCheckpoint = "ftmap/v0/get-checkpoint"
)

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

// MapCheckpoint is a commitment to a map built from the FW Log at a given size.
// The map checkpoint contains the checkpoint of the log this was built from, with
// the number of entries consumed from that input log. This allows clients to check
// they are seeing the same version of the log as the map was built from. This also
// provides information to allow verifiers of the map to confirm correct construction.
type MapCheckpoint struct {
	// LogCheckpoint is the json encoded api.LogCheckpoint.
	LogCheckpoint []byte
	LogSize       uint64
	RootHash      []byte
	Revision      uint64
}

// MapInclusionProof contains the value at the requested key and the proof to the
// requested Checkpoint.
type MapInclusionProof struct {
	Key   []byte
	Value []byte
	Proof [][]byte
}

// String returns a compact printable representation of an InclusionProof.
func (l MapInclusionProof) String() string {
	return fmt.Sprintf("{key: 0x%x, value: 0x%x, proof: %x}", l.Key, l.Value, l.Proof)
}
