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

// Package storage provides common code used by storage implementations.
package storage

// NodeCoordsToTileAddress returns the (TileLevel, TileIndex) in tile-space, and the
// (NodeLevel, NodeIndex) address within that tile of the specified tree node co-ordinates.
func NodeCoordsToTileAddress(treeLevel, treeIndex uint64) (uint64, uint64, uint, uint64) {
	tileRowWidth := uint64(1 << (8 - treeLevel%8))
	tileLevel := treeLevel / 8
	tileIndex := treeIndex / tileRowWidth
	nodeLevel := uint(treeLevel % 8)
	nodeIndex := uint64(treeIndex % tileRowWidth)

	return tileLevel, tileIndex, nodeLevel, nodeIndex
}
