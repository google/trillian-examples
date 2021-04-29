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

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// CheckpointHeader is the first line of a marshaled log checkpoint.
const CheckpointHeaderV0 = "Log Checkpoint v0"

// LogState represents the state of a serverless log
type LogState struct {
	// Size is the number of leaves in the log
	Size uint64

	// RootHash holds the Merkle tree root hash for the log at the
	// given size.
	RootHash []byte
}

// MarshalText implements golang's encoding/TextMarshaler interface, and renders
// the LogState into a UTF8 text based format:
//
// Log Checkpoint v0\n
// <decimal log size>\n
// <base64 encoded root hash>\n
func (s LogState) MarshalText() ([]byte, error) {
	r := fmt.Sprintf("%s\n%d\n%s\n", CheckpointHeaderV0, s.Size, base64.StdEncoding.EncodeToString(s.RootHash))
	return []byte(r), nil
}

// UnmarshalText implements golang's encoding/TextUnmarshaler interface, and
// populates this struct with the parsed values from the supplied text.
//
// This method reads the data serialised by the MarshalText method above,
// and it ignores any trailing.
func (s *LogState) UnmarshalText(raw []byte) error {
	text := string(raw)
	l := strings.Split(text, "\n")
	if len(l) < 3 {
		return errors.New("invalid checkpoint - too few lines")
	}
	if l[0] != CheckpointHeaderV0 {
		return errors.New("invalid checkpoint - incorrect header")
	}
	size, err := strconv.ParseUint(l[1], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid checkpoint - size invalid: %w", err)
	}
	rh, err := base64.StdEncoding.DecodeString(l[2])
	if err != nil {
		return fmt.Errorf("invalid checkpoint - invalid roothash: %w", err)
	}
	s.Size, s.RootHash = size, rh
	return nil
}

// Tile represents a subtree tile, containing inner nodes of a log tree.
type Tile struct {
	// NumLeaves is the number of entries at level 0 of this tile.
	NumLeaves uint

	// Nodes stores the log tree nodes.
	// Nodes are stored linearised using in-order traversal - this isn't completely optimal
	// in terms of storage for partial tiles, but index calculation is relatively
	// straight-forward.
	// Note that only non-ephemeral nodes are stored.
	Nodes [][]byte
}

// MarshalText implements encoding/TextMarshaller and writes out a Tile
// instance in the following format:
//
// <hash size in decimal>\n
// <num tile leaves in decimal>\n
// <Nodes[0] base64 encoded>\n
// ...
// <Nodes[n] base64 encoded>\n
func (t Tile) MarshalText() ([]byte, error) {
	b := &bytes.Buffer{}
	_, err := fmt.Fprintf(b, "%d\n%d\n", 32, t.NumLeaves)
	if err != nil {
		return nil, err
	}
	for _, n := range t.Nodes {
		_, err := fmt.Fprintf(b, "%s\n", base64.StdEncoding.EncodeToString(n))
		if err != nil {
			return nil, err
		}
	}
	return b.Bytes(), nil
}

// UnmarshalText implements encoding/TextUnmarshaler and reads tiles
// which were written by the MarshalText method above.
func (t *Tile) UnmarshalText(raw []byte) error {
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	hs, err := strconv.ParseUint(lines[0], 10, 16)
	if err != nil {
		return fmt.Errorf("unable to parse hash size: %w", err)
	}
	if hs != 32 {
		return fmt.Errorf("invalid hash size %d", hs)
	}
	numLeaves, err := strconv.ParseUint(lines[1], 10, 16)
	if err != nil {
		return fmt.Errorf("unable to parse numLeaves: %w", err)
	}
	nodes := make([][]byte, 0, numLeaves*2)
	for l := 2; l < len(lines); l++ {
		h, err := base64.StdEncoding.DecodeString(lines[l])
		if err != nil {
			return fmt.Errorf("unable to parse nodehash on line %d; %w", l, err)
		}
		nodes = append(nodes, h)
	}
	t.NumLeaves, t.Nodes = uint(numLeaves), nodes
	return nil
}

// TileNodeKey generates keys used in Tile.Nodes array.
func TileNodeKey(level uint, index uint64) uint {
	return uint(1<<(level+1)*index + 1<<level - 1)
}
