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

// Package main provides a command line tool for sequencing entries in
// a serverless log.
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/serverless/api"
	"github.com/google/trillian-examples/serverless/internal/storage/fs"
	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/merkle/rfc6962/hasher"
)

var (
	storageDir = flag.String("storage_dir", "", "Root directory to store log data.")
)

func main() {
	flag.Parse()
	h := hasher.DefaultHasher
	rf := compact.RangeFactory{h.HashChildren}
	// init storage
	st, err := fs.New(*storageDir)
	if err != nil {
		glog.Exitf("Failed to load storage: %q", err)
	}

	// fetch state
	state := st.LogState()
	baseRange, err := rf.NewRange(0, state.Size, state.Hashes)
	if err != nil {
		glog.Exitf("Failed to create range covering existing log: %q", err)
	}

	r, err := baseRange.GetRootHash(nil)
	if err != nil {
		glog.Fatalf("Invalid log state, unable to recalculate root: %q", err)
	}
	glog.Infof("Loaded state with roothash %x", r)

	tiles := make(map[string]*api.Tile)

	visitor := func(id compact.NodeID, hash []byte) {
		tileLevel := uint64(id.Level / 8)
		tileIndex := uint64(id.Index / 256)
		tileKey := tileKey(tileLevel, tileIndex)
		tile := tiles[tileKey]
		if tile == nil {
			tile, err = st.GetTile(tileLevel, tileIndex, state.Size)
			if err != nil {
				if !os.IsNotExist(err) {
					panic(err)
				}
				tile = &api.Tile{
					Nodes: make(map[string][]byte),
				}
			}
			tiles[tileKey] = tile
		}
		tile.Nodes[nodeKey(id.Level%8, id.Index%256)] = hash
	}

	// look for new sequenced entries and build tree
	newRange := rf.NewEmptyRange(state.Size)

	// write new completed subtrees
	if err := st.ScanSequenced(state.Size,
		func(seq uint64, entry []byte) error {
			lh := h.HashLeaf(entry)
			glog.Infof("new @%d: %x", seq, lh)
			// Set leafhash on zeroth level
			visitor(compact.NodeID{Level: 0, Index: seq}, lh)
			// Update range and set internal nodes
			newRange.Append(lh, visitor)
			return nil
		}); err != nil {
		glog.Exitf("Error while integrating: %q", err)
	}

	if err := baseRange.AppendRange(newRange, visitor); err != nil {
		glog.Exitf("Failed to merge new range onto existing log: %q", err)
	}

	newRoot, err := baseRange.GetRootHash(visitor)
	if err != nil {
		glog.Exitf("Failed to calculate new root hash: %q", err)
	}
	glog.Infof("New log state: size %d hash: %x", baseRange.End(), newRoot)

	for k, t := range tiles {
		l, i := splitTileKey(k)
		s := tileSize(t)
		if err := st.StoreTile(l, i, s, t); err != nil {
			glog.Exitf("Failed to store tile at level %d index %d: %q", l, i, err)
		}
	}

	newState := api.LogState{
		RootHash: newRoot,
		Size:     baseRange.End(),
		Hashes:   baseRange.Hashes(),
	}
	if err := st.UpdateState(newState); err != nil {
		glog.Exitf("Failed to update stored state: %q", err)
	}

	// done.
}

func nodeKey(level uint, index uint64) string {
	return fmt.Sprintf("%d-%d", level, index)
}

func tileSize(t *api.Tile) uint64 {
	for i := uint64(0); i < 256; i++ {
		if t.Nodes[nodeKey(0, i)] == nil {
			return i
		}
	}
	return 256
}

func tileKey(level, index uint64) string {
	return fmt.Sprintf("%d/%d", level, index)
}

func splitTileKey(s string) (uint64, uint64) {
	p := strings.Split(s, "/")
	l, err := strconv.ParseUint(p[0], 10, 64)
	if err != nil {
		panic(err)
	}
	i, err := strconv.ParseUint(p[1], 10, 64)
	if err != nil {
		panic(err)
	}
	return l, i

}
