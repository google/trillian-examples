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

// Package format provides basic support for the common checkpoint and proof
// format described by the README in this directory.
package format

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Checkpoint represents a minimal log checkpoint (STH).
type Checkpoint struct {
	// Ecosystem is the ecosystem/version string
	Ecosystem string
	// Size is the number of entries in the log at this checkpoint.
	Size uint64
	// RootHash is the hash which commits to the contents of the entire log.
	RootHash []byte
}

// Unmarshal parses the common formatted checkpoint data and stores the result
// in the Checkpoint.
//
// The supplied data is expected to begin with the following 3 lines of text:
//  - <ecosystem/version string>
//  - <decimal representation of log size>
//  - <base64 representation of root hash>
// trailing data may be present, but will be ignored.
func (c *Checkpoint) Unmarshal(data []byte) error {
	l := strings.Split(string(data), "\n")
	if len(l) < 3 {
		return errors.New("invalid checkpoint - too few lines")
	}
	eco := l[0]
	if len(eco) == 0 {
		return errors.New("invalid checkpoint - empty ecosystem")
	}
	size, err := strconv.ParseUint(l[1], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid checkpoint - size invalid: %w", err)
	}
	rh, err := base64.StdEncoding.DecodeString(l[2])
	if err != nil {
		return fmt.Errorf("invalid checkpoint - invalid roothash: %w", err)
	}
	c.Ecosystem, c.Size, c.RootHash = eco, size, rh
	return nil
}
