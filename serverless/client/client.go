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

// Package client provides client support for interacting with a serverless
// log.
//
// See the /cmd/client package in this repo for an example of using this.
package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/serverless/api"
	"github.com/google/trillian-examples/serverless/api/layout"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/merkle/logverifier"
	"golang.org/x/mod/sumdb/note"
)

// Fetcher is the signature of a function which can retrieve arbitrary files from
// a log's data storage, via whatever appropriate mechanism.
// The path parameter is relative to the root of the log storage.
//
// Note that the implementation of this MUST return (either directly or wrapped)
// an os.ErrIsNotExit when the file referenced by path does not exist, e.g. a HTTP
// based implementation MUST return this error when it receives a 404 StatusCode.
type Fetcher func(ctx context.Context, path string) ([]byte, error)

// FetchCheckpoint retrieves and opens a checkpoint from the log.
// Returns both the parsed structure and the raw serialised checkpoint.
func FetchCheckpoint(ctx context.Context, f Fetcher, v note.Verifier) (*log.Checkpoint, []byte, error) {
	cpRaw, err := f(ctx, layout.CheckpointPath)
	if err != nil {
		return nil, nil, err
	}
	n, err := note.Open(cpRaw, note.VerifierList(v))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open Checkpoint: %v", err)
	}
	cp := log.Checkpoint{}
	if _, err := cp.Unmarshal([]byte(n.Text)); err != nil {
		glog.V(1).Infof("Bad checkpoint: %q", cpRaw)
		return nil, nil, fmt.Errorf("failed to unmarshal checkpoint: %v", err)
	}
	return &cp, cpRaw, nil
}

// ProofBuilder knows how to build inclusion and consistency proofs from tiles.
// Since the tiles commit only to immutable nodes, the job of building proofs is slightly
// more complex as proofs can touch "ephemeral" nodes, so these need to be synthesized.
type ProofBuilder struct {
	cp        log.Checkpoint
	nodeCache nodeCache
	h         compact.HashFn
}

// NewProofBuilder creates a new ProofBuilder object for a given tree size.
// The returned ProofBuilder can be re-used for proofs related to a given tree size, but
// it is not thread-safe and should not be accessed concurrently.
func NewProofBuilder(ctx context.Context, cp log.Checkpoint, h compact.HashFn, f Fetcher) (*ProofBuilder, error) {
	tf := newTileFetcher(f, cp.Size)
	pb := &ProofBuilder{
		cp:        cp,
		nodeCache: newNodeCache(tf, cp.Size),
		h:         h,
	}

	hashes, err := FetchRangeNodes(ctx, cp.Size, tf)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch range nodes: %w", err)
	}
	// Create a compact range which represents the state of the log.
	r, err := (&compact.RangeFactory{Hash: h}).NewRange(0, cp.Size, hashes)
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
	if !bytes.Equal(cp.Hash, sr) {
		return nil, fmt.Errorf("invalid checkpoint hash %x, expected %x", cp.Hash, sr)
	}
	return pb, nil
}

// InclusionProof constructs an inclusion proof for the leaf at index in a tree of
// the given size.
// This function uses the passed-in function to retrieve tiles containing any log tree
// nodes necessary to build the proof.
func (pb *ProofBuilder) InclusionProof(ctx context.Context, index uint64) ([][]byte, error) {
	nodes, err := merkle.CalcInclusionProofNodeAddresses(int64(pb.cp.Size), int64(index), int64(pb.cp.Size))
	if err != nil {
		return nil, fmt.Errorf("failed to calculate inclusion proof node list: %w", err)
	}

	ret := make([][]byte, 0)
	// TODO(al) parallelise this.
	for _, n := range nodes {
		h, err := pb.nodeCache.GetNode(ctx, n.ID)
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
func (pb *ProofBuilder) ConsistencyProof(ctx context.Context, smaller, larger uint64) ([][]byte, error) {
	nodes, err := merkle.CalcConsistencyProofNodeAddresses(int64(smaller), int64(larger), int64(pb.cp.Size))
	if err != nil {
		return nil, fmt.Errorf("failed to calculate consistency proof node list: %w", err)
	}

	hashes := make([][]byte, 0)
	// TODO(al) parallelise this.
	for _, n := range nodes {
		h, err := pb.nodeCache.GetNode(ctx, n.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get node (%v): %w", n.ID, err)
		}
		hashes = append(hashes, h)
	}
	return hashes, nil
}

// FetchRangeNodes returns the set of nodes representing the compact range covering
// a log of size s.
func FetchRangeNodes(ctx context.Context, s uint64, gt GetTileFunc) ([][]byte, error) {
	nc := newNodeCache(gt, s)
	nIDs := compact.RangeNodes(0, s)
	ret := make([][]byte, len(nIDs))
	for i, n := range nIDs {
		h, err := nc.GetNode(ctx, n)
		if err != nil {
			return nil, err
		}
		ret[i] = h
	}
	return ret, nil
}

// FetchLeafHashes fetches N consecutive leaf hashes starting with the leaf at index first.
func FetchLeafHashes(ctx context.Context, f Fetcher, first, N, logSize uint64) ([][]byte, error) {
	nc := newNodeCache(newTileFetcher(f, logSize), logSize)
	ret := make([][]byte, 0, N)
	for i, seq := uint64(0), first; i < N; i, seq = i+1, seq+1 {
		nID := compact.NodeID{Level: 0, Index: seq}
		h, err := nc.GetNode(ctx, nID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch node %v: %v", nID, err)
		}
		ret = append(ret, h)
	}
	return ret, nil
}

// nodeCache hides the tiles abstraction away, and improves
// performance by caching tiles it's seen.
// Not threadsafe, and intended to be only used throughout the course
// of a single request.
type nodeCache struct {
	logSize   uint64
	ephemeral map[compact.NodeID][]byte
	tiles     map[tileKey]api.Tile
	getTile   GetTileFunc
}

// GetTileFunc is the signature of a function which knows how to fetch a
// specific tile.
type GetTileFunc func(ctx context.Context, level, index uint64) (*api.Tile, error)

// tileKey is used as a key in nodeCache's tile map.
type tileKey struct {
	tileLevel uint64
	tileIndex uint64
}

// newNodeCache creates a new nodeCache instance for a given log size.
func newNodeCache(f GetTileFunc, logSize uint64) nodeCache {
	return nodeCache{
		logSize:   logSize,
		ephemeral: make(map[compact.NodeID][]byte),
		tiles:     make(map[tileKey]api.Tile),
		getTile:   f,
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
func (n *nodeCache) GetNode(ctx context.Context, id compact.NodeID) ([]byte, error) {
	// First check for ephemeral nodes:
	if e := n.ephemeral[id]; len(e) != 0 {
		return e, nil
	}
	// Otherwise look in fetched tiles:
	tileLevel, tileIndex, nodeLevel, nodeIndex := layout.NodeCoordsToTileAddress(uint64(id.Level), uint64(id.Index))
	tKey := tileKey{tileLevel, tileIndex}
	t, ok := n.tiles[tKey]
	if !ok {
		tile, err := n.getTile(ctx, tileLevel, tileIndex)
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

// newTileFetcher returns a GetTileFunc based on the passed in Fetcher and log size.
func newTileFetcher(f Fetcher, logSize uint64) GetTileFunc {
	return func(ctx context.Context, level, index uint64) (*api.Tile, error) {
		tileSize := layout.PartialTileSize(level, index, logSize)
		p := filepath.Join(layout.TilePath("", level, index, tileSize))
		t, err := f(ctx, p)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("failed to read tile at %q: %w", p, err)
			}
			return nil, err
		}

		var tile api.Tile
		if err := tile.UnmarshalText(t); err != nil {
			return nil, fmt.Errorf("failed to parse tile: %w", err)
		}
		return &tile, nil
	}
}

// LookupIndex fetches the leafhash->seq mapping file from the log, and returns
// its parsed contents.
func LookupIndex(ctx context.Context, f Fetcher, lh []byte) (uint64, error) {
	p := filepath.Join(layout.LeafPath("", lh))
	sRaw, err := f(ctx, p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, fmt.Errorf("leafhash unknown: %w", err)
		}
		return 0, fmt.Errorf("failed to fetch leafhash->seq file: %w", err)
	}
	return strconv.ParseUint(string(sRaw), 16, 64)
}

// GetLeaf fetches the raw contents committed to at a given leaf index.
func GetLeaf(ctx context.Context, f Fetcher, i uint64) ([]byte, error) {
	p := filepath.Join(layout.SeqPath("", i))
	sRaw, err := f(ctx, p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("leaf index %d not found: %w", i, err)
		}
		return nil, fmt.Errorf("failed to fetch leaf index %d: %w", i, err)
	}
	return sRaw, nil
}

// LogStateTracker represents a client-side view of a target log's state.
// This tracker handles verification that updates to the tracked log state are
// consistent with previously seen states.
type LogStateTracker struct {
	Hasher   hashers.LogHasher
	Verifier logverifier.LogVerifier
	Fetcher  Fetcher

	// LatestConsistentRaw holds the raw bytes of the latest proven-consistent
	// LogState seen by this tracker.
	LatestConsistentRaw []byte

	// LatestConsistent is the deserialised form of LatestConsistentRaw
	LatestConsistent log.Checkpoint
	CpSigVerifier    note.Verifier
}

// NewLogStateTracker creates a newly initialised tracker.
// If a serialised LogState representation is provided then this is used as the
// initial tracked state, otherwise a log state is fetched from the target log.
func NewLogStateTracker(ctx context.Context, f Fetcher, h hashers.LogHasher, checkpointRaw []byte, nV note.Verifier) (LogStateTracker, error) {
	ret := LogStateTracker{
		Fetcher:          f,
		Hasher:           h,
		Verifier:         logverifier.New(h),
		LatestConsistent: log.Checkpoint{},
		CpSigVerifier:    nV,
	}
	if len(checkpointRaw) > 0 {
		ret.LatestConsistentRaw = checkpointRaw
		if _, err := ret.LatestConsistent.Unmarshal(checkpointRaw); err != nil {
			return ret, err
		}
		return ret, nil
	}
	return ret, ret.Update(ctx)
}

// ErrInconsistency should be returned when there has been an error proving consistency
// between log states.
// The raw log state representations are included as-returned by the target log, this
// ensures that evidence of inconsistent log updates are available to the caller of
// the method(s) returning this error.
type ErrInconsistency struct {
	SmallerRaw []byte
	LargerRaw  []byte
	Proof      [][]byte

	Wrapped error
}

func (e ErrInconsistency) Unwrap() error {
	return e.Wrapped
}

func (e ErrInconsistency) Error() string {
	return fmt.Sprintf("log consistency check failed: %s", e.Wrapped)
}

// Update attempts to update the local view of the target log's state.
// If a more recent logstate is found, this method will attempt to prove
// that it is consistent with the local state before updating the tracker's
// view.
func (lst *LogStateTracker) Update(ctx context.Context) error {
	c, cRaw, err := FetchCheckpoint(ctx, lst.Fetcher, lst.CpSigVerifier)
	if err != nil {
		return err
	}
	if lst.LatestConsistent.Size > 0 {
		if c.Size > lst.LatestConsistent.Size {
			builder, err := NewProofBuilder(ctx, *c, lst.Hasher.HashChildren, lst.Fetcher)
			if err != nil {
				return fmt.Errorf("failed to create proof builder: %w", err)
			}
			p, err := builder.ConsistencyProof(ctx, lst.LatestConsistent.Size, c.Size)
			if err != nil {
				return err
			}
			glog.V(1).Infof("Built consistency proof %x", p)
			if err := lst.Verifier.VerifyConsistencyProof(int64(lst.LatestConsistent.Size), int64(c.Size), lst.LatestConsistent.Hash, c.Hash, p); err != nil {
				return ErrInconsistency{
					SmallerRaw: lst.LatestConsistentRaw,
					LargerRaw:  cRaw,
					Proof:      p,
					Wrapped:    err,
				}
			}
		}
	}
	lst.LatestConsistentRaw, lst.LatestConsistent = cRaw, *c
	return nil
}

// CheckConsistency is a wapper function which simplifies verifying consistency between two or more checkpoints.
func CheckConsistency(ctx context.Context, h hashers.LogHasher, f Fetcher, cp []log.Checkpoint) error {
	sort.Slice(cp, func(i, j int) bool {
		return cp[i].Size < cp[j].Size
	})
	pb, err := NewProofBuilder(ctx, cp[len(cp)-1], h.HashChildren, f)
	if err != nil {
		return fmt.Errorf("failed to create proofbuilder: %v", err)
	}

	lv := logverifier.New(h)

	// Go through list of checkpoints pairwise, checking consistency
	var a, b log.Checkpoint
	for i := 0; i < len(cp)-1; i, a, b = i+1, cp[i], cp[i+1] {
		if a.Size == b.Size {
			if bytes.Equal(a.Hash, b.Hash) {
				continue
			}
			return fmt.Errorf("two checkpoints with same size (%d) but different hashes (%x vs %x)", a.Size, a.Hash, b.Hash)
		}
		if a.Size > 0 {
			cp, err := pb.ConsistencyProof(ctx, a.Size, b.Size)
			if err != nil {
				return fmt.Errorf("failed to fetch consistency between sizes %d, %d: %v", a.Size, b.Size, err)
			}
			if err := lv.VerifyConsistencyProof(int64(a.Size), int64(b.Size), a.Hash, b.Hash, cp); err != nil {
				return fmt.Errorf("invalid consistency proof between sizes %d, %d: %v", a.Size, b.Size, err)
			}
		}
	}
	return nil
}
