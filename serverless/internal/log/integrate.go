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

// Package log provides the underlying functionality for managing log data
// structures.
package log

import (
	"errors"

	"github.com/google/trillian-examples/serverless/api"
	"github.com/google/trillian/merkle/hashers"
)

// Storage represents the set of functions needed by the log tooling.
type Storage interface {
	// LogState returns the current state of the stored log.
	LogState() api.LogState

	// UpdateState stores a newly updated log state.
	UpdateState(newState api.LogState) error

	// Sequence assigns sequence numbers to the passed in entry.
	Sequence(leafhash []byte, leaf []byte) error
}

// Integrate adds sequenced but not-yet-included entries into the tree state.
func Integrate(st Storage, h hashers.LogHasher) error {
	return errors.New("unimplemented")
}
