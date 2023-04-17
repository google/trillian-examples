// Copyright 2023 Google LLC. All Rights Reserved.
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

// Package api provides the API endpoints for the distributor.
package api

const (
	// HTTPGetCheckpointN is the path of the URL to get a checkpoint with
	// at least N signatures.  The placeholders are:
	//  * first position is for the logID (an alphanumeric string)
	//  * second position is the number of signatures required
	HTTPGetCheckpointN = "/distributor/v0/logs/%s/checkpoint.%s"
	// HTTPCheckpointByWitness is the path of the URL to the latest checkpoint
	// for a given log by a given witness. This can take GET requests to fetch
	// the latest version, and PUT requests to update the latest checkpoint.
	//  * first position is for the logID (an alphanumeric string)
	//  * second position is the witness short name (alpha string)
	HTTPCheckpointByWitness = "/distributor/v0/logs/%s/byWitness/%s/checkpoint"
	// HTTPGetLogs is the path of the URL to get a list of all logs the
	// distributor is aware of.
	HTTPGetLogs = "/distributor/v0/logs"
)
