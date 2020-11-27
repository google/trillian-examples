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

// verify confirms that all of the entries in a go.sum file are committed to
// by the verifiable map created in this demo.
package main

import (
	"bufio"
	"bytes"
	"crypto"
	"flag"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/golang/glog"
	"github.com/google/trillian/experimental/batchmap/tilepb"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/coniks"

	"github.com/google/trillian-examples/experimental/batchmap/sumdb/mapdb"

	_ "github.com/mattn/go-sqlite3"
)

const hash = crypto.SHA512_256

var (
	sumFile      = flag.String("sum_file", "", "go.sum file to check for integrity.")
	mapDB        = flag.String("map_db", "", "sqlite DB containing the map tiles.")
	treeID       = flag.Int64("tree_id", 12345, "The ID of the tree. Used as a salt in hashing.")
	prefixStrata = flag.Int("prefix_strata", 2, "The number of strata of 8-bit strata before the final strata.")
)

func main() {
	flag.Parse()

	if *mapDB == "" {
		glog.Exitf("No map_dir provided")
	}
	if *sumFile == "" {
		glog.Exitf("No sum_file provided")
	}

	tiledb, err := mapdb.NewTileDB(*mapDB)
	if err != nil {
		glog.Exitf("Failed to open map DB at %q: %v", *mapDB, err)
	}
	var rev int
	if rev, err = tiledb.MaxRevision(); err != nil {
		glog.Exitf("No revisions found in map DB at %q: %v", *mapDB, err)
	}

	// Open the go.sum file for reading a line at a time.
	file, err := os.Open(*sumFile)
	if err != nil {
		glog.Exitf("failed to read file %q: %q", *sumFile, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)

	var root []byte
	var count int
	// Confirm that each entry in the go.sum file is committed to by the map.
	// NOTE:
	// This is optimized for clarity of demonstrating inclusion, not for performance.
	// This code leads to a lot of duplicate I/O and tile calculation, particularly
	// for the higher tiles in the map (e.g. the root tile will be read and verified
	// for every single line in the go.sum file).
	for scanner.Scan() {
		line := scanner.Text()
		split := strings.LastIndex(line, " ")
		key := line[:split]
		expectedString := line[split+1:]
		glog.V(1).Infof("checking key %q value %q", key, expectedString)
		newRoot, err := checkInclusion(tiledb, rev, key, expectedString)
		if err != nil {
			glog.Exitf("inclusion check failed for key %q value %q: %q", key, expectedString, err)
		}
		if root != nil && !bytes.Equal(root, newRoot) {
			glog.Exitf("map root changed while verifying file")
		}
		root = newRoot
		count++
	}
	glog.Infof("verified %d entries committed to by map root %x", count, root)
}

// checkInclusion confirms that the key & value are committed to by the map in the given
// directory, and returns the computed and confirmed root hash that commits to this.
func checkInclusion(tiledb *mapdb.TileDB, rev int, key, value string) ([]byte, error) {
	// Determine the key/value we expect to find.
	// Note that the map tiles do not contain raw values, but commitments to the values.
	// If the map needs to return the values to clients then it is recommended that the
	// map operator uses a Content Addressable Store to store these values.
	h := hash.New()
	h.Write([]byte(key))
	keyPath := h.Sum(nil)

	expectedValueHash := coniks.Default.HashLeaf(*treeID, keyPath, []byte(value))

	// Read the tiles required for this check from disk.
	tiles, err := getTilesForKey(tiledb, rev, keyPath)
	if err != nil {
		return nil, fmt.Errorf("couldn't load tiles: %v", err)
	}

	// Perform the verification.
	// 1) Start at the leaf tile and check the key/value.
	// 2) Compute the merkle root of the leaf tile
	// 3) Check the computed root matches that reported in the tile
	// 4) Check this root value is the key/value of the tile above.
	// 5) Rinse and repeat until we reach the tree root.
	hs2 := merkle.NewHStar2(*treeID, coniks.Default)
	needPath, needValue := keyPath, expectedValueHash

	for i := *prefixStrata; i >= 0; i-- {
		tile := tiles[i]
		// Check the prefix of what we are looking for matches the tile's path.
		if got, want := tile.Path, needPath[:len(tile.Path)]; !bytes.Equal(got, want) {
			return nil, fmt.Errorf("wrong tile found at index %d: got %x, want %x", i, got, want)
		}
		// Leaf paths within a tile are within the scope of the tile, so we can
		// drop the prefix from the expected path now we have verified it.
		needLeafPath := needPath[len(tile.Path):]

		// Identify the leaf we need, and convert all leaves to the format needed for hashing.
		var leaf *tilepb.TileLeaf
		hs2Leaves := make([]*merkle.HStar2LeafHash, len(tile.Leaves))
		for j, l := range tile.Leaves {
			if bytes.Equal(l.Path, needLeafPath) {
				leaf = l
			}
			hs2Leaves[j] = toHStar2(tile.Path, l)
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
		root, err := hs2.HStar2Nodes(tile.Path, 8*len(leaf.Path), hs2Leaves, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to hash tile %x: %v", tile.Path, err)
		}
		if !bytes.Equal(root, tile.RootHash) {
			return nil, fmt.Errorf("wrong root hash for tile %x: got %x, calculated %x", tile.Path, tile.RootHash, root)
		}

		// Make the next iteration of the loop check that the tile above this has the
		// root value of this tile stored as the value at the expected leaf index.
		needPath, needValue = tile.Path, root
	}

	// If we get here then we have proved that the value was correct and that the map
	// root commits to this value. Any other user with the same map root must see the
	// same value under the same key we have checked.
	glog.V(2).Infof("key %s found at path %x, with value %q (%x) committed to by map root %x", key, keyPath, value, expectedValueHash, needValue)
	return needValue, nil
}

// getTilesForKey loads the tiles on the path from the root to the given leaf.
func getTilesForKey(tiledb *mapdb.TileDB, rev int, key []byte) ([]*tilepb.Tile, error) {
	tiles := make([]*tilepb.Tile, *prefixStrata+1)
	for i := 0; i <= *prefixStrata; i++ {
		tilePath := key[0:i]
		tile, err := tiledb.Tile(rev, tilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read tile %x @ revision %d: %v", tilePath, rev, err)
		}
		tiles[i] = tile
	}
	return tiles, nil
}

// toHStar2 converts a TileLeaf into the equivalent structure for HStar2.
func toHStar2(prefix []byte, l *tilepb.TileLeaf) *merkle.HStar2LeafHash {
	// In hstar2 all paths need to be 256 bit (32 bytes)
	leafIndexBs := make([]byte, 32)
	copy(leafIndexBs, prefix)
	copy(leafIndexBs[len(prefix):], l.Path)
	return &merkle.HStar2LeafHash{
		Index:    new(big.Int).SetBytes(leafIndexBs),
		LeafHash: l.Hash,
	}
}
