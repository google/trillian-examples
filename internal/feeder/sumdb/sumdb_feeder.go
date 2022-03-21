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

// Package sumdb implements a feeder for the Go SumDB log.
package sumdb

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/internal/feeder"
	"github.com/google/trillian-examples/serverless/config"
	"github.com/google/trillian-examples/sumdbaudit/client"
	"github.com/transparency-dev/merkle/compact"
	"github.com/transparency-dev/merkle/rfc6962"
	"golang.org/x/mod/sumdb/tlog"

	i_note "github.com/google/trillian-examples/internal/note"
)

const (
	tileHeight    = 8
	leavesPerTile = 1 << tileHeight
)

var (
	rf = &compact.RangeFactory{
		Hash: rfc6962.DefaultHasher.HashChildren,
	}
)

func FeedLog(ctx context.Context, l config.Log, w feeder.Witness, c *http.Client, interval time.Duration) error {
	sdb := client.NewSumDB(tileHeight, l.PublicKey, l.URL, c)
	logSigV, err := i_note.NewVerifier(l.PublicKeyType, l.PublicKey)
	if err != nil {
		return err
	}

	fetchProof := func(ctx context.Context, from, to log.Checkpoint) ([][]byte, error) {
		broker := newTileBroker(to.Size, sdb.TileHashes)

		required := compact.RangeNodes(from.Size, to.Size)
		proof := make([][]byte, 0, len(required))
		for _, n := range required {
			i, r := convertToSumDBTiles(n)
			t, err := broker.tile(i)
			if err != nil {
				return nil, fmt.Errorf("failed to lookup %s: %v", i, err)
			}
			h := t.hash(r)
			proof = append(proof, h)
		}
		return proof, nil
	}

	fetchCheckpoint := func(_ context.Context) ([]byte, error) {
		sdbcp, err := sdb.LatestCheckpoint()
		if err != nil {
			return nil, fmt.Errorf("failed to get latest checkpoint: %v", err)
		}
		return sdbcp.Raw, nil

	}

	opts := feeder.FeedOpts{
		LogID:           l.ID,
		LogOrigin:       l.Origin,
		FetchCheckpoint: fetchCheckpoint,
		FetchProof:      fetchProof,
		LogSigVerifier:  logSigV,
		Witness:         w,
	}

	return feeder.Run(ctx, interval, opts)
}

// convertToSumDBTiles takes a NodeID pointing to a node within the overall log,
// and returns the tile coordinate that it will be found in, along with the nodeID
// that references the node within that tile.
func convertToSumDBTiles(n compact.NodeID) (tileIndex, compact.NodeID) {
	sdbLevel := n.Level / tileHeight
	rLevel := uint(n.Level % tileHeight)

	// The last leaf that n commits to.
	lastLeaf := (1<<n.Level)*(n.Index+1) - 1
	// How many proper leaves are committed to by a tile at sdbLevel.
	tileLeaves := 1 << ((1 + sdbLevel) * tileHeight)

	sdbOffset := lastLeaf / uint64(tileLeaves)
	rLeaves := lastLeaf % uint64(tileLeaves)
	rIndex := rLeaves / (1 << n.Level)

	return tileIndex{
		level:  int(sdbLevel),
		offset: int(sdbOffset),
	}, compact.NewNodeID(rLevel, rIndex)
}

// tileIndex is the location of a tile.
// We could have used compact.NodeID, but that would overload its meaning;
// with this usage tileIndex points to a tile, and NodeID points to a node.
type tileIndex struct {
	level, offset int
}

func (i tileIndex) String() string {
	return fmt.Sprintf("T(%d,%d)", i.level, i.offset)
}

type tile struct {
	leaves [][]byte
}

func newTile(hs []tlog.Hash) tile {
	leaves := make([][]byte, len(hs))
	for i, h := range hs {
		h := h
		leaves[i] = h[:]
	}
	return tile{
		leaves: leaves,
	}
}

// hash returns the hash of the subtree within this tile identified by n.
// Note that the coordinate system starts with the bottom left of the tile
// being (0,0), no matter where this tile appears within the overall log.
func (t tile) hash(n compact.NodeID) []byte {
	r := rf.NewEmptyRange(0)

	left := n.Index << uint64(n.Level)
	right := (n.Index + 1) << uint64(n.Level)

	if len(t.leaves) < int(right) {
		panic(fmt.Sprintf("index %d out of range of %d leaves", right, len(t.leaves)))
	}
	for _, l := range t.leaves[left:right] {
		r.Append(l[:], nil)
	}
	root, err := r.GetRootHash(nil)
	if err != nil {
		panic(err)
	}
	return root
}

// tileBroker takes requests for tiles and returns the appropriate tile for the tree size.
// This will cache results for the lifetime of the broker, and it will ensure that requests
// for incomplete tiles are requested as partial tiles.
type tileBroker struct {
	pts    map[tileIndex]int
	ts     map[tileIndex]tile
	lookup func(level, offset, partial int) ([]tlog.Hash, error)
}

func newTileBroker(size uint64, lookup func(level, offset, partial int) ([]tlog.Hash, error)) tileBroker {
	pts := make(map[tileIndex]int)
	l := 0
	t := size / leavesPerTile
	o := size % leavesPerTile
	for {
		i := tileIndex{l, int(t)}
		pts[i] = int(o)
		if t == 0 {
			break
		}
		o = t % leavesPerTile
		t = t / leavesPerTile
		l++
	}
	return tileBroker{
		pts:    pts,
		ts:     make(map[tileIndex]tile),
		lookup: lookup,
	}
}

func (tb *tileBroker) tile(i tileIndex) (tile, error) {
	if t, ok := tb.ts[i]; ok {
		return t, nil
	}
	partial := tb.pts[i]
	glog.V(2).Infof("Looking up %s (partial=%d) from remote", i, partial)
	hs, err := tb.lookup(i.level, i.offset, partial)
	if err != nil {
		return tile{}, fmt.Errorf("lookup failed: %v", err)
	}
	t := newTile(hs)
	tb.ts[i] = t
	return t, nil
}
