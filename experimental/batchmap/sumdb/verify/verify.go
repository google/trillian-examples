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
	"os"
	"strings"

	"github.com/golang/glog"

	"github.com/google/trillian-examples/experimental/batchmap/sumdb/mapdb"
	"github.com/google/trillian-examples/experimental/batchmap/sumdb/verification"

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
	var logRoot []byte
	if rev, logRoot, _, err = tiledb.LatestRevision(); err != nil {
		glog.Exitf("No revisions found in map DB at %q: %v", *mapDB, err)
	}

	mv := verification.NewMapVerifier(tiledb.Tile, *prefixStrata, *treeID, hash)

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
		newRoot, err := mv.CheckInclusion(rev, key, []byte(expectedString))
		if err != nil {
			glog.Exitf("inclusion check failed for key %q value %q: %q", key, expectedString, err)
		}
		if root != nil && !bytes.Equal(root, newRoot) {
			glog.Exitf("map root changed while verifying file")
		}
		root = newRoot
		count++
	}
	glog.Infof("Verified %d entries committed to by map rev %d root %x. Log checkpoint:\n%s", count, rev, root, logRoot)
}
