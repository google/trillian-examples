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

import "context"

const (
	// HTTPGetCheckpoint is the path of the URL to get a checkpoint.
	HTTPGetCheckpoint = "witness/v0/get-checkpoint"
	// HTTPUpdate is the path of the URL to update to a new checkpoint.
	HTTPUpdate = "witness/v0/update"
)

// UpdateRequest encodes the inputs to the witness Update function: a logID
// string, (raw) checkpoint byte slice, and consistency proof (slice of slices).
type UpdateRequest struct {
	Context    context.Context
	LogID      string
	Checkpoint []byte
	Proof      [][]byte
}
