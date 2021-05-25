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

import (
	"github.com/google/trillian-examples/formats/log"
)

// UpdatePackage represents an "OTA" update bundle.
type UpdatePackage struct {
	// FirmwareImage is the actual firmware image itself.
	FirmwareImage []byte
	// ProofBundle holds the various artifacts required to validate the firmware image.
	// It should be a serialised JSON form of the ProofBundle structure.
	ProofBundle []byte
}

// ProofBundle contains the manifest and associated proofs for a given firmware image.
type ProofBundle struct {
	// ManifestStatement is the json representation of an `api.SignedStatement` struct.
	ManifestStatement []byte
	// Checkpoint must represent a tree which includes the ManifestStatement.
	Checkpoint log.SignedCheckpoint
	// InclusionProof is a proof to Checkpoint for ManifestStatement.
	InclusionProof InclusionProof
}
