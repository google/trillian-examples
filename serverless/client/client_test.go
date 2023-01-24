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

package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/trillian-examples/serverless/api"
	"github.com/transparency-dev/formats/log"
	"github.com/transparency-dev/merkle/compact"
	"github.com/transparency-dev/merkle/rfc6962"
)

var (
	// Built using serverless/testdata/build_log.sh
	testCheckpoints = []log.Checkpoint{
		{
			Size: 1,
			Hash: b64("0Nc2CrefWKseHj/mStd+LqC8B+NrX0btIiPt2SmN+ek="),
		},
		{
			Size: 2,
			Hash: b64("T1X2GdkhUjV3iyufF9b0kVsWFxIU0VI4EpNml2Teci4="),
		},
		{
			Size: 3,
			Hash: b64("Wqx3HImawpLnS/Gv4ubjAvi1WIOy0b8Ze0amvqbavKk="),
		},
		{
			Size: 4,
			Hash: b64("zY1lN35vrXYAPixXSd59LsU29xUJtuW4o2dNNg5Y2Co="),
		},
		{
			Size: 5,
			Hash: b64("gy5gl3aksFyiCO95a/1vLXz88A3dRq+0l9Sxte8ZqZQ="),
		},
		{
			Size: 6,
			Hash: b64("a6sWvsc2eEzmj72vah7mZ5dwFltivehh2b11qwlp5Jg="),
		},
		{
			Size: 7,
			Hash: b64("IrSXADBqJ7EQoUODSDKROySgNveeL6CFhik2w/+fS7U="),
		},
		{
			Size: 14,
			Hash: b64("SvCd38yNade7xEPY1a/aAc1M3A2AHYVF8lIiUnsH1ao="),
		},
		{
			Size: 15,
			Hash: b64("rKbDipCvhuX1GZ7g5BBe8sA6BbJ7ja/1nk427v383cs="),
		},
	}
)

func b64(r string) []byte {
	ret, err := base64.StdEncoding.DecodeString(r)
	if err != nil {
		panic(err)
	}
	return ret
}

func TestCheckConsistency(t *testing.T) {
	ctx := context.Background()

	h := rfc6962.DefaultHasher

	f := func(_ context.Context, p string) ([]byte, error) {
		path := filepath.Join("../testdata/log", p)
		return os.ReadFile(path)
	}
	for _, test := range []struct {
		desc    string
		cp      []log.Checkpoint
		wantErr bool
	}{
		{
			desc: "2 CP",
			cp: []log.Checkpoint{
				testCheckpoints[2],
				testCheckpoints[5],
			},
		}, {
			desc: "5 CP",
			cp: []log.Checkpoint{
				testCheckpoints[0],
				testCheckpoints[2],
				testCheckpoints[3],
				testCheckpoints[5],
				testCheckpoints[6],
			},
		}, {
			desc: "big CPs",
			cp: []log.Checkpoint{
				testCheckpoints[3],
				testCheckpoints[7],
				testCheckpoints[8],
			},
		}, {
			desc: "Identical CP",
			cp: []log.Checkpoint{
				testCheckpoints[0],
				testCheckpoints[0],
				testCheckpoints[0],
				testCheckpoints[0],
			},
		}, {
			desc: "Identical CP pairs",
			cp: []log.Checkpoint{
				testCheckpoints[0],
				testCheckpoints[0],
				testCheckpoints[5],
				testCheckpoints[5],
			},
		}, {
			desc: "Out of order",
			cp: []log.Checkpoint{
				testCheckpoints[5],
				testCheckpoints[2],
				testCheckpoints[0],
				testCheckpoints[3],
			},
		}, {
			desc:    "no checkpoints",
			cp:      []log.Checkpoint{},
			wantErr: true,
		}, {
			desc: "one checkpoint",
			cp: []log.Checkpoint{
				testCheckpoints[3],
			},
			wantErr: true,
		}, {
			desc: "two inconsistent CPs",
			cp: []log.Checkpoint{
				{
					Size: 2,
					Hash: []byte("This is a banana"),
				},
				testCheckpoints[4],
			},
			wantErr: true,
		}, {
			desc: "Inconsistent",
			cp: []log.Checkpoint{
				testCheckpoints[5],
				testCheckpoints[2],
				{
					Size: 4,
					Hash: []byte("This is a banana"),
				},
				testCheckpoints[3],
			},
			wantErr: true,
		}, {
			desc: "Inconsistent - clashing CPs",
			cp: []log.Checkpoint{
				{
					Size: 2,
					Hash: []byte("This is a banana"),
				},
				{
					Size: 2,
					Hash: []byte("This is NOT a banana"),
				},
			},
			wantErr: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			err := CheckConsistency(ctx, h, f, test.cp)
			if gotErr := err != nil; gotErr != test.wantErr {
				t.Fatalf("wantErr: %t, got %v", test.wantErr, err)
			}
		})
	}
}

func TestNodeCacheHandlesInvalidRequest(t *testing.T) {
	ctx := context.Background()
	wantBytes := []byte("one")
	f := func(_ context.Context, _, _ uint64) (*api.Tile, error) {
		return &api.Tile{
			Nodes: [][]byte{wantBytes},
		}, nil
	}

	// Large tree, but we're emulating skew since f, above, will return a tile which only knows about 1
	// leaf.
	nc := newNodeCache(f, 10)

	if got, err := nc.GetNode(ctx, compact.NewNodeID(0, 0)); err != nil {
		t.Errorf("got %v, want no error", err)
	} else if !bytes.Equal(got, wantBytes) {
		t.Errorf("got %v, want %v", got, wantBytes)
	}

	if _, err := nc.GetNode(ctx, compact.NewNodeID(0, 1)); err == nil {
		t.Error("got no error, want error because ID is out of range")
	}
}
