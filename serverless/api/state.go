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

// Package api contains the "public" API/artifacts of the serverless log.
package api

// LogState represents the state of a serverless log
type LogState struct {
	// Size is the number of leaves in the log
	Size uint64

	// SHA256 log root, RFC6962 flavour.
	RootHash []byte

	// Hashes are the roots of the minimal set of perfect subtrees contained by this log.
	Hashes [][]byte
}
