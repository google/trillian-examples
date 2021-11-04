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

package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/google/trillian/merkle/compact"
	"golang.org/x/mod/sumdb/tlog"
)

func TestConvertToSumDBTiles(t *testing.T) {
	for _, test := range []struct {
		n     compact.NodeID
		wantT tileIndex
		wantN compact.NodeID
	}{
		{
			n:     compact.NewNodeID(0, 0),
			wantT: tileIndex{0, 0},
			wantN: compact.NewNodeID(0, 0),
		},
		{
			n:     compact.NewNodeID(0, 255),
			wantT: tileIndex{0, 0},
			wantN: compact.NewNodeID(0, 255),
		},
		{
			n:     compact.NewNodeID(0, 256),
			wantT: tileIndex{0, 1},
			wantN: compact.NewNodeID(0, 0),
		},
		{
			n:     compact.NewNodeID(1, 127),
			wantT: tileIndex{0, 0},
			wantN: compact.NewNodeID(1, 127),
		},
		{
			n:     compact.NewNodeID(1, 128),
			wantT: tileIndex{0, 1},
			wantN: compact.NewNodeID(1, 0),
		},
		{
			n:     compact.NewNodeID(8, 0),
			wantT: tileIndex{1, 0},
			wantN: compact.NewNodeID(0, 0),
		},
		{
			n:     compact.NewNodeID(8, 1),
			wantT: tileIndex{1, 0},
			wantN: compact.NewNodeID(0, 1),
		},
		{
			n:     compact.NewNodeID(22, 0),
			wantT: tileIndex{2, 0},
			wantN: compact.NewNodeID(6, 0),
		},
		{
			n:     compact.NewNodeID(8, 24338),
			wantT: tileIndex{1, 95},
			wantN: compact.NewNodeID(0, 18),
		},
	} {
		t.Run(string(fmt.Sprintf("(%d,%d)", test.n.Level, test.n.Index)), func(t *testing.T) {
			gotT, gotN := convertToSumDBTiles(test.n)
			if gotT != test.wantT {
				t.Errorf("got %d, want %d", gotT, test.wantT)
			}
			if gotN != test.wantN {
				t.Errorf("got %d, want %d", gotN, test.wantN)
			}
		})
	}
}

func TestTileBroker(t *testing.T) {
	for _, test := range []struct {
		desc       string
		size       uint64
		i          tileIndex
		wantLeaves int
	}{
		{
			desc:       "big tree complete tile",
			size:       12345678,
			i:          tileIndex{0, 0},
			wantLeaves: 256,
		},
		{
			desc:       "small tree partial tile",
			size:       3,
			i:          tileIndex{0, 0},
			wantLeaves: 3,
		},
		{
			desc:       "medium tree partial leaf tile",
			size:       257,
			i:          tileIndex{0, 1},
			wantLeaves: 1,
		},
		{
			desc:       "medium tree partial tile in upper row",
			size:       257,
			i:          tileIndex{1, 0},
			wantLeaves: 1,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			var lookupTimes int
			lookup := func(level, offset, partial int) ([]tlog.Hash, error) {
				n := partial
				if n == 0 {
					n = 256
				}
				r := make([]tlog.Hash, n)
				for i := 0; i < n; i++ {
					r[i] = tlog.RecordHash([]byte(fmt.Sprintf("%d %d: %d", level, offset, i)))
				}
				lookupTimes++
				return r, nil
			}
			broker := newTileBroker(test.size, lookup)

			tile, err := broker.tile(test.i)
			if err != nil {
				t.Fatalf("failed to get tile: %v", err)
			}
			if got, want := len(tile.leaves), test.wantLeaves; got != want {
				t.Errorf("got %d leaves, wanted %d", got, want)
			}
			broker.tile(test.i)
			if lookupTimes != 1 {
				t.Errorf("broker cache not working")
			}
		})
	}
}

func TestTile(t *testing.T) {
	for _, test := range []struct {
		desc     string
		size     int
		nodeID   compact.NodeID
		wantHash []byte
	}{
		{
			desc:     "first leaf",
			size:     8,
			nodeID:   compact.NewNodeID(0, 0),
			wantHash: mustDecodeHex("1bb97dcc21635d47e2663efdfd0a174686d98dd701352dd2cd06e8b43fd3d305"),
		},
		{
			desc:     "last leaf",
			size:     8,
			nodeID:   compact.NewNodeID(0, 7),
			wantHash: mustDecodeHex("4b4711d056b2278392c231fd41858adea8ca893ad0c7048f57da2682002845fe"),
		},
		{
			desc:     "interior node",
			size:     8,
			nodeID:   compact.NewNodeID(0, 3),
			wantHash: mustDecodeHex("58bd1496e1684aac9201c2e687ee7ae4f51c96a8b0d81ef3583628b93d3cd345"),
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ls := make([]tlog.Hash, test.size)
			for i := 0; i < test.size; i++ {
				ls[i] = tlog.RecordHash([]byte(fmt.Sprintf("leaf %d", i)))
			}
			tile := newTile(ls)

			hash := tile.hash(test.nodeID)
			if !bytes.Equal(hash, test.wantHash) {
				t.Errorf("mismatched hash: wanted %x got %x", test.wantHash, hash)
			}
		})
	}
}

func mustDecodeHex(h string) []byte {
	r, err := hex.DecodeString(h)
	if err != nil {
		panic(err)
	}
	return r
}
