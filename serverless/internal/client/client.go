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
	"bytes"
	"fmt"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/serverless/api"
	"github.com/google/trillian-examples/serverless/internal/storage"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/compact"
)

// GetTileFunc is the signature of a function which knows how to fetch log subtree tiles.
type GetTileFunc func(level, index, logSize uint64) (*api.Tile, error)

// ProofBuilder knows how to build inclusion and consistency proofs from tiles.
// Since the tiles commit only to immutable nodes, the job of building proofs is slightly
// more complex as proofs can touch "ephemeral" nodes, so these need to be synthesized.
type ProofBuilder struct {
	ls        api.LogState
	nodeCache nodeCache
	h         compact.HashFn
}

// NewProofBuilder creates a new ProofBuilder object for a given tree size.
// The returned ProofBuilder can be re-used for proofs related to a given tree size, but
// it is not thread-safe and should not be accessed concurrently.
func NewProofBuilder(s api.LogState, h compact.HashFn, f GetTileFunc) (*ProofBuilder, error) {
	pb := &ProofBuilder{
		ls:        s,
		nodeCache: newNodeCache(f),
		h:         h,
	}

	r, err := (&compact.RangeFactory{Hash: h}).NewRange(0, s.Size, s.Hashes)
	if err != nil {
		return nil, err
	}

	sr, err := r.GetRootHash(pb.nodeCache.SetEphemeralNode)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(s.RootHash, sr) {
		return nil, fmt.Errorf("invalid logstate roothash %x, expected %x", s.RootHash, sr)
	}
	return pb, nil
}

// InclusionProof constructs an inclusion proof for the leaf at index in a tree of
// the given size.
// This function uses the passed-in function to retrieve tiles containing any log tree
// nodes necessary to build the proof.
func (pb *ProofBuilder) InclusionProof(index uint64) ([][]byte, error) {
	nodes, err := merkle.CalcInclusionProofNodeAddresses(int64(pb.ls.Size), int64(index), int64(pb.ls.Size))
	if err != nil {
		return nil, fmt.Errorf("failed to calculate inclusion proof node list: %w", err)
	}

	ret := make([][]byte, 0)
	// TODO(al) parallelise
	for _, n := range nodes {
		h, err := pb.nodeCache.GetNode(n.ID, pb.ls.Size)
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
func (pb *ProofBuilder) ConsistencyProof(smaller, larger uint64) ([][]byte, error) {
	nodes, err := merkle.CalcConsistencyProofNodeAddresses(int64(smaller), int64(larger), int64(pb.ls.Size))
	if err != nil {
		return nil, fmt.Errorf("failed to calculate consistency proof node list: %w", err)
	}

	hashes := make([][]byte, 0)
	// TODO(al) parallelise
	for _, n := range nodes {
		h, err := pb.nodeCache.GetNode(n.ID, pb.ls.Size)
		if err != nil {
			return nil, fmt.Errorf("failed to get node (%v): %w", n.ID, err)
		}
		hashes = append(hashes, h)
	}
	return hashes, nil
}

// nodeCache hides the tiles abstraction away, and improves
// performance by caching tiles it's seen.
// Not threadsafe, and intended to be only used throughout the course
// of a single request.
type nodeCache struct {
	ephemeral map[string][]byte
	tiles     map[string]api.Tile
	getTile   GetTileFunc
}

func newNodeCache(f GetTileFunc) nodeCache {
	return nodeCache{
		ephemeral: make(map[string][]byte),
		tiles:     make(map[string]api.Tile),
		getTile:   f,
	}
}

// tileKey creates keys to be used internally by nodeCache.
func tileKey(l uint64, i uint64) string {
	return fmt.Sprintf("%d/%d", l, i)
}

// SetEphemeralNode stored a derived "ephemeral" tree node.
func (n *nodeCache) SetEphemeralNode(id compact.NodeID, h []byte) {
	n.ephemeral[tileKey(uint64(id.Level), id.Index)] = h
}

// GetNode returns the internal log tree node hash for the specified node ID.
// A previously set ephemeral node will be returned if id matches, otherwise
// the tile containing the requested node will be fetched and cached, and the
// node hash returned.
func (n *nodeCache) GetNode(id compact.NodeID, logSize uint64) ([]byte, error) {
	// First check for ephemeral nodes:
	if e := n.ephemeral[tileKey(uint64(id.Level), id.Index)]; len(e) != 0 {
		return e, nil
	}
	// Otherwise look in fetched tiles:
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
