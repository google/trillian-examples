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

// TileNodeKey generates keys used in Tile.Nodes array.
func TileNodeKey(level uint, index uint64) uint {
	return uint(1<<(level+1)*index + 1<<level - 1)
}
