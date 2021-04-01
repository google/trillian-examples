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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/trillian-examples/serverless/api"
	"github.com/google/trillian-examples/serverless/internal/layout"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/compact"
)

// FetcherFunc is the signature of a function which can retrieve arbitrary files from
// a log's data storage, via whatever appropriate mechanism.
// The path parameter is relative to the root of the log storage.
type FetcherFunc func(path string) ([]byte, error)

func GetLogState(f FetcherFunc) (*api.LogState, error) {
	sRaw, err := f(layout.StatePath)
	if err != nil {
		return nil, err
	}
	var ls api.LogState
	if err := json.Unmarshal(sRaw, &ls); err != nil {
		return nil, fmt.Errorf("failed to unmarshal logstate: %w", err)
	}
	return &ls, nil
}

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
func NewProofBuilder(s api.LogState, h compact.HashFn, f FetcherFunc) (*ProofBuilder, error) {
	pb := &ProofBuilder{
		ls:        s,
		nodeCache: newNodeCache(f),
		h:         h,
	}

	// Create a compact range which represents the state of the log.
	r, err := (&compact.RangeFactory{Hash: h}).NewRange(0, s.Size, s.Hashes)
	if err != nil {
		return nil, err
	}

	// Recreate the root hash so that:
	// a) we validate the self-integrity of the log state, and
	// b) we calculate (and cache) and ephemeral nodes present in the tree,
	//    this is important since they could be required by proofs.
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
	// TODO(al) parallelise this.
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
	// TODO(al) parallelise this.
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
	ephemeral map[compact.NodeID][]byte
	tiles     map[tileKey]api.Tile
	fetcher   FetcherFunc
}

// tileKey is used as a key in nodeCache's tile map.
type tileKey struct {
	tileLevel uint64
	tileIndex uint64
}

// newNodeCache creates a new nodeCache instance.
func newNodeCache(f FetcherFunc) nodeCache {
	return nodeCache{
		ephemeral: make(map[compact.NodeID][]byte),
		tiles:     make(map[tileKey]api.Tile),
		fetcher:   f,
	}
}

// SetEphemeralNode stored a derived "ephemeral" tree node.
func (n *nodeCache) SetEphemeralNode(id compact.NodeID, h []byte) {
	n.ephemeral[id] = h
}

// GetNode returns the internal log tree node hash for the specified node ID.
// A previously set ephemeral node will be returned if id matches, otherwise
// the tile containing the requested node will be fetched and cached, and the
// node hash returned.
func (n *nodeCache) GetNode(id compact.NodeID, logSize uint64) ([]byte, error) {
	// First check for ephemeral nodes:
	if e := n.ephemeral[id]; len(e) != 0 {
		return e, nil
	}
	// Otherwise look in fetched tiles:
	tileLevel, tileIndex, nodeLevel, nodeIndex := layout.NodeCoordsToTileAddress(uint64(id.Level), uint64(id.Index))
	tKey := tileKey{tileLevel, tileIndex}
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

// getTile retrieves and parses the tile containing the specified node.
func (n *nodeCache) getTile(level, index, logSize uint64) (*api.Tile, error) {
	tileSize := layout.PartialTileSize(level, index, logSize)
	p := filepath.Join(layout.TilePath("", level, index, tileSize))
	t, err := n.fetcher(p)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("failed to read tile at %q: %w", p, err)
		}
		return nil, err
	}

	var tile api.Tile
	if err := json.Unmarshal(t, &tile); err != nil {
		return nil, fmt.Errorf("failed to parse tile: %w", err)
	}
	return &tile, nil
}
