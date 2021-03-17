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

// Package client is a client for the serverless log.
package client

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/serverless/api"
	"github.com/google/trillian-examples/serverless/internal/storage"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/compact"
)

// GetTileFunc is the signature of a function which knows how to fetch log subtree tiles.
type GetTileFunc func(level, index, logSize uint64) (*api.Tile, error)

// InclusionProof constructs an inclusion proof for the leaf at index in a tree of
// the given size.
// This function uses the passed-in function to retrieve tiles containing any log tree
// nodes necessary to build the proof.
func InclusionProof(index, size uint64, f GetTileFunc) ([][]byte, error) {
	nodes, err := merkle.CalcInclusionProofNodeAddresses(int64(size), int64(index), int64(size))
	if err != nil {
		return nil, fmt.Errorf("failed to calculate inclusion proof node list: %w", err)
	}

	nc := newNodeCache(f)
	ret := make([][]byte, 0)
	// TODO(al) parallelise
	for _, n := range nodes {
		h, err := nc.GetNode(n.ID, size)
		if err != nil {
			return nil, fmt.Errorf("failed to get node (%v): %w", n.ID, err)
		}
		ret = append(ret, h)
	}
	return ret, nil
}

// ConsistencyProof constructs a consistency proof between the two passed in tree sizes.
// This function uses the passed-in function to retrieve tiles containing any log tree
// nodes necessary to build the proof.
func ConsistencyProof(smaller, larger uint64, f GetTileFunc) ([][]byte, error) {
	return nil, errors.New("unimpl")
}

// nodeCache hides the tiles abstraction away, and improves
// performance by caching tiles it's seen.
// Not threadsafe, and intended to be only used throughout the course
// of a single request.
type nodeCache struct {
	tiles   map[string]api.Tile
	getTile GetTileFunc
}

func newNodeCache(f GetTileFunc) nodeCache {
	return nodeCache{
		tiles:   make(map[string]api.Tile),
		getTile: f,
	}
}

// tileKey creates keys to be used internally by nodeCache.
func tileKey(l uint64, i uint64) string {
	return fmt.Sprintf("%d/%d", l, i)
}

// GetNode returns the internal log tree node hash for the specified node ID.
func (n *nodeCache) GetNode(id compact.NodeID, logSize uint64) ([]byte, error) {
	tileLevel, tileIndex, nodeLevel, nodeIndex := storage.NodeCoordsToTileAddress(uint64(id.Level), uint64(id.Index))
	tKey := tileKey(tileLevel, tileIndex)
	t, ok := n.tiles[tKey]
	if !ok {
		tile, err := n.getTile(tileLevel, tileIndex, logSize)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch tile: %w", err)
		}
		t = *tile
		n.tiles[tKey] = *tile
	}
	node := t.Nodes[api.TileNodeKey(nodeLevel, nodeIndex)]
	if node == nil {
		return nil, fmt.Errorf("node %v (tile coords [%d,%d]/[%d,%d]) unknown", id, tileLevel, tileIndex, nodeLevel, nodeIndex)
	}
	return node, nil
}
