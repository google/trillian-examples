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

// feeder polls the sumdb log and pushes the results to a generic witness.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/sumdbaudit/client"
	"github.com/google/trillian-examples/witness/golang/client/http"
	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/merkle/rfc6962"
	"golang.org/x/mod/sumdb/note"
	"golang.org/x/mod/sumdb/tlog"
)

var (
	vkey         = flag.String("k", "sum.golang.org+033de0ae+Ac4zctda0e5eza+HJyk9SxEdh+s3Ux18htTTAD8OuAn8", "key")
	origin       = flag.String("origin", "go.sum database tree", "The expected first origin log (first line of the checkpoint)")
	witness      = flag.String("w", "", "The endpoint of the witness HTTP REST API")
	witnessKey   = flag.String("wk", "", "The public key of the witness")
	logID        = flag.String("lid", "", "The ID of the log within the witness")
	pollInterval = flag.Duration("poll", 10*time.Second, "How quickly to poll the sumdb to get updates")

	rf = &compact.RangeFactory{
		Hash: rfc6962.DefaultHasher.HashChildren,
	}
)

const (
	tileHeight    = 8
	leavesPerTile = 1 << tileHeight
)

func main() {
	flag.Parse()
	ctx := context.Background()
	sdb := client.NewSumDB(tileHeight, *vkey)
	var w http.Witness
	if wURL, err := url.Parse(*witness); err != nil {
		glog.Exitf("Failed to parse witness URL: %v", err)
	} else {
		w = http.Witness{
			URL:      wURL,
			Verifier: mustCreateVerifier(*witnessKey),
		}
	}

	wcp := &log.Checkpoint{}
	if wcpRaw, err := w.GetLatestCheckpoint(ctx, *logID); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			glog.Exitf("Failed to get witness checkpoint: %v", err)
		}
	} else {
		wcp, _, err = log.ParseCheckpoint(*origin, w.Verifier, []note.Verifier{}, wcpRaw)
		if err != nil {
			glog.Exitf("Failed to open CP: %v", err)
		}
	}

	feeder := feeder{
		wcp: wcp,
		sdb: sdb,
		w:   w,
	}

	tik := time.NewTicker(*pollInterval)
	for {
		glog.V(2).Infof("Tick: start feedOnce (witness size %d)", feeder.wcp.Size)
		if err := feeder.feedOnce(ctx); err != nil {
			glog.Warningf("Failed to feed: %v", err)
		}
		glog.V(2).Infof("Tick: feedOnce complete (witness size %d)", feeder.wcp.Size)

		select {
		case <-ctx.Done():
			return
		case <-tik.C:
		}
	}
}

// feeder encapsulates the main logic and state of the feeder.
// The residual logic outside of this should simply be initialization and
// orchestration with a timer.
// Note that feeder maintains its own state that represents the witness state.
// This is optimized for the case where this is the only feeder for this log to
// the witness. If the witness state is updated by another entity, then the
// number of successful updates from this feeder will drop. On the other hand,
// if this is the only feeder then it avoids making requests to get the latest
// checkpoint from the witness when it isn't changing.
type feeder struct {
	wcp *log.Checkpoint
	sdb *client.SumDBClient
	w   http.Witness
}

// feedOnce gets the latest checkpoint from the SumDB server, and if this is more
// recent than the witness state then it will construct a compact range proof by
// requesting the minimal set of tiles, and then update the witness with the new
// checkpoint. Finally, it will update the feeder's view of the witness state.
func (f *feeder) feedOnce(ctx context.Context) error {
	sdbcp, err := f.sdb.LatestCheckpoint()
	if err != nil {
		return fmt.Errorf("failed to get latest checkpoint: %v", err)
	}
	if int64(f.wcp.Size) >= sdbcp.N {
		glog.V(1).Infof("Witness size %d >= SumDB size %d - nothing to do", f.wcp.Size, sdbcp.N)
		return nil
	}

	glog.Infof("Updating witness from size %d to %d", f.wcp.Size, sdbcp.N)

	broker := newTileBroker(sdbcp.N, f.sdb.TileHashes)

	required := compact.RangeNodes(f.wcp.Size, uint64(sdbcp.N))
	proof := make([][]byte, 0, len(required))
	for _, n := range required {
		i, r := convertToSumDBTiles(n)
		t, err := broker.tile(i)
		if err != nil {
			return fmt.Errorf("failed to lookup %s: %v", i, err)
		}
		h := t.hash(r)
		proof = append(proof, h)
	}

	wcpRaw, err := f.w.Update(ctx, *logID, sdbcp.Raw, proof)

	if err != nil && !errors.Is(err, http.ErrCheckpointTooOld) {
		return fmt.Errorf("failed to update checkpoint: %v", err)
	}

	// An optimization would be to immediately retry the Update if we get
	// http.ErrCheckpointTooOld. For now, we'll update the local state and
	// retry only the next time this method is called.
	f.wcp, _, err = log.ParseCheckpoint(*origin, f.w.Verifier, []note.Verifier{}, wcpRaw)
	if err != nil {
		return fmt.Errorf("failed to parse checkpoint: %v", err)
	}
	return nil
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

func newTileBroker(size int64, lookup func(level, offset, partial int) ([]tlog.Hash, error)) tileBroker {
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

func mustCreateVerifier(pub string) note.Verifier {
	v, err := note.NewVerifier(pub)
	if err != nil {
		glog.Exitf("Failed to create signature verifier from %q: %v", pub, err)
	}
	return v
}
