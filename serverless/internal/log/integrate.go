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
	"fmt"
	"os"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/serverless/api"
	"github.com/google/trillian-examples/serverless/internal/storage"
	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/merkle/hashers"
)

// Storage represents the set of functions needed by the log tooling.
type Storage interface {
	// GetTile returns the tile at the given level & index.
	GetTile(level, index, logSize uint64) (*api.Tile, error)

	// StoreTile stores the tile at the given level & index.
	StoreTile(level, index uint64, tile *api.Tile) error

	// LogState returns the current state of the stored log.
	LogState() api.LogState

	// UpdateState stores a newly updated log state.
	UpdateState(newState api.LogState) error

	// ScanSequenced calls f for each sequenced log entry >= begin.
	// It should stop scanning if the call to f returns an error.
	ScanSequenced(begin uint64, f func(seq uint64, entry []byte) error) (uint64, error)

	// Sequence assigns sequence numbers to the passed in entry.
	Sequence(leafhash []byte, leaf []byte) error
}

// Integrate adds sequenced but not-yet-included entries into the tree state.
func Integrate(st Storage, h hashers.LogHasher) error {
	rf := compact.RangeFactory{Hash: h.HashChildren}

	// fetch state
	state := st.LogState()
	baseRange, err := rf.NewRange(0, state.Size, state.Hashes)
	if err != nil {
		return fmt.Errorf("failed to create range covering existing log: %q", err)
	}

	r, err := baseRange.GetRootHash(nil)
	if err != nil {
		return fmt.Errorf("invalid log state, unable to recalculate root: %q", err)
	}
	glog.Infof("Loaded state with roothash %x", r)

	tiles := make(map[string]*api.Tile)

	visitor := func(id compact.NodeID, hash []byte) {
		tileLevel := uint64(id.Level / 8)
		tileIndex := uint64(id.Index >> (8 - id.Level%8))
		tileKey := storage.TileKey(tileLevel, tileIndex)
		tile := tiles[tileKey]
		if tile == nil {
			created := false
			tile, err = st.GetTile(tileLevel, tileIndex, state.Size)
			if err != nil {
				if !os.IsNotExist(err) {
					panic(err)
				}
				created = true
				tile = &api.Tile{
					Nodes: make(map[string][]byte),
				}
			}
			glog.V(1).Infof("GetTile: %s new: %v", tileKey, created)
			tiles[tileKey] = tile
		}
		tile.Nodes[api.TileNodeKey(id.Level%8, id.Index%256)] = hash
	}

	// look for new sequenced entries and build tree
	newRange := rf.NewEmptyRange(state.Size)

	// write new completed subtrees
	n, err := st.ScanSequenced(state.Size,
		func(seq uint64, entry []byte) error {
			lh := h.HashLeaf(entry)
			glog.V(2).Infof("new @%d: %x", seq, lh)
			// Set leafhash on zeroth level
			visitor(compact.NodeID{Level: 0, Index: seq}, lh)
			// Update range and set internal nodes
			newRange.Append(lh, visitor)
			return nil
		})
	if err != nil {
		return fmt.Errorf("error while integrating: %q", err)
	}
	if n == 0 {
		glog.Infof("Nothing to do.")
		// Nothing to do, nothing done.
		return nil
	}

	if err := baseRange.AppendRange(newRange, visitor); err != nil {
		return fmt.Errorf("failed to merge new range onto existing log: %q", err)
	}

	newRoot, err := baseRange.GetRootHash(nil)
	if err != nil {
		return fmt.Errorf("failed to calculate new root hash: %q", err)
	}
	glog.Infof("New log state: size %d hash: %x", baseRange.End(), newRoot)

	for k, t := range tiles {
		l, i := storage.SplitTileKey(k)
		if err := st.StoreTile(l, i, t); err != nil {
			return fmt.Errorf("failed to store tile at level %d index %d: %q", l, i, err)
		}
	}

	newState := api.LogState{
		RootHash: newRoot,
		Size:     baseRange.End(),
		Hashes:   baseRange.Hashes(),
	}
	if err := st.UpdateState(newState); err != nil {
		return fmt.Errorf("failed to update stored state: %q", err)
	}
	return nil
}
