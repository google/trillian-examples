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
	"context"
	"fmt"
	"os"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/serverless/api"
	"github.com/google/trillian-examples/serverless/api/layout"
	"github.com/google/trillian-examples/serverless/client"
	"github.com/transparency-dev/merkle"
	"github.com/transparency-dev/merkle/compact"
)

// Storage represents the set of functions needed by the log tooling.
type Storage interface {
	// GetTile returns the tile at the given level & index.
	GetTile(level, index, logSize uint64) (*api.Tile, error)

	// StoreTile stores the tile at the given level & index.
	StoreTile(level, index uint64, tile *api.Tile) error

	// WriteCheckpoint stores a newly updated log checkpoint.
	WriteCheckpoint(ctx context.Context, newCPRaw []byte) error

	// Sequence assigns sequence numbers to the passed in entry.
	// Returns the assigned sequence number for the leafhash.
	//
	// If a duplicate leaf is sequenced the storage implementation may return
	// the sequence number associated with an earlier instance, along with a
	// os.ErrDupeLeaf error.
	Sequence(ctx context.Context, leafhash []byte, leaf []byte) (uint64, error)

	// ScanSequenced calls f for each contiguous sequenced log entry >= begin.
	// It should stop scanning if the call to f returns an error.
	ScanSequenced(ctx context.Context, begin uint64, f func(seq uint64, entry []byte) error) (uint64, error)
}

// Integrate adds all sequenced entries greater than checkpoint.Size into the tree.
// Returns an updated Checkpoint, or an error.
func Integrate(ctx context.Context, checkpoint log.Checkpoint, st Storage, h merkle.LogHasher) (*log.Checkpoint, error) {
	getTile := func(l, i uint64) (*api.Tile, error) {
		return st.GetTile(l, i, checkpoint.Size)
	}

	hashes, err := client.FetchRangeNodes(ctx, checkpoint.Size, func(_ context.Context, l, i uint64) (*api.Tile, error) {
		return getTile(l, i)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch compact range nodes: %w", err)
	}

	rf := compact.RangeFactory{Hash: h.HashChildren}
	baseRange, err := rf.NewRange(0, checkpoint.Size, hashes)
	if err != nil {
		return nil, fmt.Errorf("failed to create range covering existing log: %w", err)
	}

	// Initialise a compact range representation, and verify the stored state.
	r, err := baseRange.GetRootHash(nil)
	if err != nil {
		return nil, fmt.Errorf("invalid log state, unable to recalculate root: %w", err)
	}

	glog.Infof("Loaded state with roothash %x", r)

	// Create a new compact range which represents the update to the tree
	newRange := rf.NewEmptyRange(checkpoint.Size)
	tc := tileCache{m: make(map[tileKey]*api.Tile), getTile: getTile}
	n, err := st.ScanSequenced(context.Background(),
		checkpoint.Size,
		func(seq uint64, entry []byte) error {
			lh := h.HashLeaf(entry)
			// Set leafhash on zeroth level
			tc.Visit(compact.NodeID{Level: 0, Index: seq}, lh)
			// Update range and set internal nodes
			newRange.Append(lh, tc.Visit)
			return nil
		})
	if err != nil {
		return nil, fmt.Errorf("error while integrating: %w", err)
	}
	if n == 0 {
		glog.Infof("Nothing to do.")
		// Nothing to do, nothing done.
		return nil, nil
	}

	// Merge the update range into the old tree
	if err := baseRange.AppendRange(newRange, tc.Visit); err != nil {
		return nil, fmt.Errorf("failed to merge new range onto existing log: %w", err)
	}

	// Calculate the new root hash - don't pass in the tileCache visitor here since
	// this will construct any emphemeral nodes and we do not want to store those.
	newRoot, err := baseRange.GetRootHash(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate new root hash: %w", err)
	}

	// All calculation is now complete, all that remains is to store the new
	// tiles and updated log state.
	glog.Infof("New log state: size 0x%x hash: %x", baseRange.End(), newRoot)

	for k, t := range tc.m {
		if err := st.StoreTile(k.level, k.index, t); err != nil {
			return nil, fmt.Errorf("failed to store tile at level %d index %d: %w", k.level, k.index, err)
		}
	}

	// Finally, return a new checkpoint struct to the caller, so they can sign &
	// persist it.
	// Since the sequencing is already completed (by the sequence tool), any
	// failures to write/update the tree are idempotent and can be safely
	// re-tried with a subsequent run of this method. Also, until WriteCheckpoint
	// is successfully invoked, clients have no root hash for a larger tree so
	// it's meaningless for them to attempt to construct inclusion/consistency
	// proofs.
	newCP := log.Checkpoint{
		Hash: newRoot,
		Size: baseRange.End(),
	}
	return &newCP, nil
}

// tileKey is a level/index key for the tile cache below.
type tileKey struct {
	level uint64
	index uint64
}

// tileCache is a simple cache for storing the newly created tiles produced by
// the integration of new leaves into the tree.
//
// Calls to Visit will cause the map of tiles to become filled with the set of
// `dirty` tiles which need to be flushed back to disk to preserve the updated
// tree state.
//
// Note that by itself, this cache does not update any on-disk state.
type tileCache struct {
	m map[tileKey]*api.Tile

	getTile func(level, index uint64) (*api.Tile, error)
}

// Visit should be called once for each newly set non-ephemeral node in the
// tree.
//
// If the tile containing id has not been seen before, this method will fetch
// it from disk (or create a new empty in-memory tile if it doesn't exist), and
// update it by setting the node corresponding to id to the value hash.
func (tc tileCache) Visit(id compact.NodeID, hash []byte) {
	tileLevel, tileIndex, nodeLevel, nodeIndex := layout.NodeCoordsToTileAddress(uint64(id.Level), uint64(id.Index))
	tileKey := tileKey{level: tileLevel, index: tileIndex}
	tile := tc.m[tileKey]
	var err error
	if tile == nil {
		// We haven't see this tile before, so try to fetch it from disk
		created := false
		tile, err = tc.getTile(tileLevel, tileIndex)
		if err != nil {
			if !os.IsNotExist(err) {
				panic(err)
			}
			// This is a brand new tile.
			created = true
			tile = &api.Tile{
				Nodes: make([][]byte, 0, 256*2),
			}
		}
		glog.V(1).Infof("GetTile: %v new: %v", tileKey, created)
		tc.m[tileKey] = tile
	}
	// Update the tile with the new node hash.
	idx := api.TileNodeKey(nodeLevel, nodeIndex)
	if l := uint(len(tile.Nodes)); idx >= l {
		tile.Nodes = append(tile.Nodes, make([][]byte, idx-l+1)...)
	}
	tile.Nodes[idx] = hash
	// Update the number of 'tile leaves', if necessary.
	if nodeLevel == 0 && nodeIndex >= uint64(tile.NumLeaves) {
		tile.NumLeaves = uint(nodeIndex + 1)
	}
}
