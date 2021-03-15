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

// Package verification contains verifiers for clients of the map to confirm
// entries are committed to.
package verification

import (
	"bytes"
	"crypto"
	"fmt"

	"github.com/google/trillian/experimental/batchmap"
	"github.com/google/trillian/merkle/coniks/hasher"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/merkle/smt"
	"github.com/google/trillian/storage/tree"
)

// TileFetch gets the tile at the specified path in the given map revision.
// There is currently an assumption that this is very fast and thus it looks
// up tiles one at a time. This can be replaced with a batch version if that
// assumption is invalidated (e.g. this method triggers network operations).
type TileFetch func(revision int, path []byte) (*batchmap.Tile, error)

// MapVerifier verifies inclusion of key/values in a map.
type MapVerifier struct {
	tileFetch    TileFetch
	prefixStrata int
	treeID       int64
	hash         crypto.Hash
}

// NewMapVerifier returns a MapVerifier for the map at the given location and with the
// configuration provided.
func NewMapVerifier(tileFetch TileFetch, prefixStrata int, treeID int64, hash crypto.Hash) *MapVerifier {
	return &MapVerifier{
		tileFetch:    tileFetch,
		prefixStrata: prefixStrata,
		treeID:       treeID,
		hash:         hash,
	}
}

// CheckInclusion confirms that the key & value are committed to by the map in the given
// directory, and returns the computed and confirmed root hash that commits to this.
func (v *MapVerifier) CheckInclusion(rev int, key string, value []byte) ([]byte, error) {
	// Determine the key/value we expect to find.
	// Note that the map tiles do not contain raw values, but commitments to the values.
	// If the map needs to return the values to clients then it is recommended that the
	// map operator uses a Content Addressable Store to store these values.
	h := v.hash.New()
	h.Write([]byte(key))
	keyPath := h.Sum(nil)
	leafID := tree.NewNodeID2(string(keyPath), uint(len(keyPath)*8))

	expectedValueHash := hasher.Default.HashLeaf(v.treeID, leafID, value)

	// Read the tiles required for this check from disk.
	tiles, err := v.getTilesForKey(rev, keyPath)
	if err != nil {
		return nil, fmt.Errorf("couldn't load tiles: %v", err)
	}

	// Perform the verification.
	// 1) Start at the leaf tile and check the key/value.
	// 2) Compute the merkle root of the leaf tile
	// 3) Check the computed root matches that reported in the tile
	// 4) Check this root value is the key/value of the tile above.
	// 5) Rinse and repeat until we reach the tree root.
	et := emptyTree{treeID: v.treeID, hasher: hasher.Default}
	needPath, needValue := keyPath, expectedValueHash

	for i := v.prefixStrata; i >= 0; i-- {
		tile := tiles[i]
		// Check the prefix of what we are looking for matches the tile's path.
		if got, want := tile.Path, needPath[:len(tile.Path)]; !bytes.Equal(got, want) {
			return nil, fmt.Errorf("wrong tile found at index %d: got %x, want %x", i, got, want)
		}
		// Leaf paths within a tile are within the scope of the tile, so we can
		// drop the prefix from the expected path now we have verified it.
		needLeafPath := needPath[len(tile.Path):]

		// Identify the leaf we need, and convert all leaves to the format needed for hashing.
		var leaf *batchmap.TileLeaf
		nodes := make([]smt.Node, len(tile.Leaves))
		for j, l := range tile.Leaves {
			if bytes.Equal(l.Path, needLeafPath) {
				leaf = l
			}
			nodes[j] = toNode(tile.Path, l)
		}

		// Confirm we found the leaf we needed, and that it had the value we expected.
		if leaf == nil {
			return nil, fmt.Errorf("couldn't find expected leaf %x in tile %x", needLeafPath, tile.Path)
		}
		if !bytes.Equal(leaf.Hash, needValue) {
			return nil, fmt.Errorf("wrong leaf value in tile %x, leaf %x: got %x, want %x", tile.Path, leaf.Path, leaf.Hash, needValue)
		}

		// Hash this tile given its leaf values, and confirm that the value we compute
		// matches the value reported in the tile.
		hs, err := smt.NewHStar3(nodes, et.hasher.HashChildren,
			uint(len(tile.Path)+len(leaf.Path))*8, uint(len(tile.Path))*8)
		if err != nil {
			return nil, fmt.Errorf("failed to create HStar3 for tile %x: %v", tile.Path, err)
		}
		res, err := hs.Update(et)
		if err != nil {
			return nil, fmt.Errorf("failed to hash tile %x: %v", tile.Path, err)
		} else if got, want := len(res), 1; got != want {
			return nil, fmt.Errorf("wrong number of roots for tile %x: got %v, want %v", tile.Path, got, want)
		}
		if got, want := res[0].Hash, tile.RootHash; !bytes.Equal(got, want) {
			return nil, fmt.Errorf("wrong root hash for tile %x: got %x, calculated %x", tile.Path, got, want)
		}

		// Make the next iteration of the loop check that the tile above this has the
		// root value of this tile stored as the value at the expected leaf index.
		needPath, needValue = tile.Path, res[0].Hash
	}

	return needValue, nil
}

// getTilesForKey loads the tiles on the path from the root to the given leaf.
func (v *MapVerifier) getTilesForKey(rev int, key []byte) ([]*batchmap.Tile, error) {
	tiles := make([]*batchmap.Tile, v.prefixStrata+1)
	for i := 0; i <= v.prefixStrata; i++ {
		tilePath := key[0:i]
		tile, err := v.tileFetch(rev, tilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read tile %x @ revision %d: %v", tilePath, rev, err)
		}
		tiles[i] = tile
	}
	return tiles, nil
}

// toNode converts a TileLeaf into the equivalent Node for HStar3.
func toNode(prefix []byte, l *batchmap.TileLeaf) smt.Node {
	path := make([]byte, 0, len(prefix)+len(l.Path))
	path = append(append(path, prefix...), l.Path...)
	return smt.Node{
		ID:   tree.NewNodeID2(string(path), uint(len(path))*8),
		Hash: l.Hash,
	}
}

// emptyTree is a NodeAccessor for an empty tree with the given ID.
type emptyTree struct {
	treeID int64
	hasher hashers.MapHasher
}

func (e emptyTree) Get(id tree.NodeID2) ([]byte, error) {
	return e.hasher.HashEmpty(e.treeID, id), nil
}

func (e emptyTree) Set(id tree.NodeID2, hash []byte) {}
