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

// versions lists the versions for a module and verifies this in the map.
package main

import (
	"crypto"
	"flag"
	"fmt"
	"regexp"

	"github.com/golang/glog"
	"golang.org/x/mod/sumdb/tlog"

	"github.com/google/trillian-examples/experimental/batchmap/sumdb/mapdb"
	"github.com/google/trillian-examples/experimental/batchmap/sumdb/verification"
	"github.com/transparency-dev/merkle/compact"

	_ "github.com/mattn/go-sqlite3"
)

const hash = crypto.SHA512_256

var (
	module       = flag.String("module", "", "module to get versions for.")
	mapDB        = flag.String("map_db", "", "sqlite DB containing the map tiles.")
	treeID       = flag.Int64("tree_id", 12345, "The ID of the tree. Used as a salt in hashing.")
	prefixStrata = flag.Int("prefix_strata", 2, "The number of strata of 8-bit strata before the final strata.")
	showAll      = flag.Bool("all", false, "Only release versions are shown by default, but setting this flag will also show ephemeral versions.")
)

func main() {
	flag.Parse()

	if *mapDB == "" {
		glog.Exitf("No map_db provided")
	}
	if *module == "" {
		glog.Exitf("No module provided")
	}

	tiledb, err := mapdb.NewTileDB(*mapDB)
	if err != nil {
		glog.Exitf("Failed to open map DB at %q: %v", *mapDB, err)
	}
	var rev int
	if rev, _, _, err = tiledb.LatestRevision(); err != nil {
		glog.Exitf("No revisions found in map DB at %q: %v", *mapDB, err)
	}

	versions, err := tiledb.Versions(rev, *module)
	if err != nil {
		glog.Exitf("Failed to list versions for %q: %v", *module, err)
	}

	rf := &compact.RangeFactory{
		// This needs to be the same function used in the log construction.
		Hash: func(left, right []byte) []byte {
			var lHash, rHash tlog.Hash
			copy(lHash[:], left)
			copy(rHash[:], right)
			thash := tlog.NodeHash(lHash, rHash)
			return thash[:]
		},
	}
	logRange := rf.NewEmptyRange(0)
	for _, v := range versions {
		h := tlog.RecordHash([]byte(v))
		logRange.Append(h[:], nil)
	}
	logRoot, err := logRange.GetRootHash(nil)
	if err != nil {
		glog.Exitf("Failed to calculate expected log root: %v", err)
	}

	mv := verification.NewMapVerifier(tiledb.Tile, *prefixStrata, *treeID, hash)
	mr, err := mv.CheckInclusion(rev, *module, logRoot)
	if err != nil {
		glog.Exitf("Failed to verify inclusion: %v", err)
	}

	releaseRegex := regexp.MustCompile(`^v\d+.\d+.\d+$`)
	var versionString string
	var skipped int
	for _, v := range versions {
		if *showAll || releaseRegex.MatchString(v) {
			versionString = fmt.Sprintf("%s\n * %s", versionString, v)
		} else {
			skipped++
		}
	}
	if skipped > 0 {
		versionString = fmt.Sprintf("%s\n(%d omitted non-release versions)", versionString, skipped)
	}
	glog.Infof("Verified versions for %q in map with root %x: %s", *module, mr, versionString)
}
