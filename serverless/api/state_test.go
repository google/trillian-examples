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

// package api_test contains tests for the api package.
package api_test

import (
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian-examples/serverless/api"
)

func TestNodeKey(t *testing.T) {
	for _, test := range []struct {
		level uint
		index uint64
		want  uint
	}{
		{
			level: 0,
			index: 0,
			want:  0,
		}, {
			level: 1,
			index: 0,
			want:  1,
		}, {
			level: 1,
			index: 1,
			want:  5,
		},
	} {
		t.Run(fmt.Sprintf("level %d, index %d", test.level, test.index), func(t *testing.T) {
			if got, want := api.TileNodeKey(test.level, test.index), test.want; got != want {
				t.Fatalf("got %d want %d", got, want)
			}
		})
	}
}

func emptyHashes(n uint) [][]byte {
	r := make([][]byte, n)
	for i := range r {
		r[i] = make([]byte, 32)
	}
	return r
}

func TestMarshalTileRoundtrip(t *testing.T) {
	tile := api.Tile{
		Nodes: make([][]byte, 0, 256),
	}
	for i := 1; i < 256; i++ {
		tile.NumLeaves = uint(i)
		idx := api.TileNodeKey(0, uint64(i-1))
		if l := uint(len(tile.Nodes)); idx >= l {
			tile.Nodes = append(tile.Nodes, emptyHashes(idx-l+1)...)
		}
		// Fill in the leaf index
		rand.Read(tile.Nodes[idx])

		raw, err := tile.MarshalText()
		if err != nil {
			t.Fatalf("MarshalText() = %v", err)
		}

		tile2 := api.Tile{}
		if err := tile2.UnmarshalText(raw); err != nil {
			t.Fatalf("UnmarshalText() = %v", err)
		}

		if diff := cmp.Diff(tile, tile2); len(diff) != 0 {
			t.Fatalf("Got tile with diff: %s", diff)
		}
	}
}

func TestMarshalLogState(t *testing.T) {
	for _, test := range []struct {
		s    api.LogState
		want string
	}{
		{
			s: api.LogState{
				Size:     123,
				RootHash: []byte("bananas"),
			},
			want: "Log Checkpoint v0\n123\nYmFuYW5hcw==\n",
		}, {
			s: api.LogState{
				Size:     9944,
				RootHash: []byte("the view from the tree tops is great!"),
			},
			want: "Log Checkpoint v0\n9944\ndGhlIHZpZXcgZnJvbSB0aGUgdHJlZSB0b3BzIGlzIGdyZWF0IQ==\n",
		},
	} {
		t.Run(string(test.s.RootHash), func(t *testing.T) {
			got, err := test.s.MarshalText()
			if err != nil {
				t.Fatalf("MarshalText = %v", err)
			}
			if string(got) != test.want {
				t.Fatalf("MarshalText = %q, want %q", got, test.want)
			}
		})
	}
}

func TestUnmarshalLogState(t *testing.T) {
	for _, test := range []struct {
		desc    string
		m       string
		want    api.LogState
		wantErr bool
	}{
		{
			desc: "valid one",
			m:    "Log Checkpoint v0\n123\nYmFuYW5hcw==\n",
			want: api.LogState{
				Size:     123,
				RootHash: []byte("bananas"),
			},
		}, {
			desc: "valid two",
			m:    "Log Checkpoint v0\n9944\ndGhlIHZpZXcgZnJvbSB0aGUgdHJlZSB0b3BzIGlzIGdyZWF0IQ==\n",
			want: api.LogState{
				Size:     9944,
				RootHash: []byte("the view from the tree tops is great!"),
			},
		}, {
			desc: "valid with trailing data",
			m:    "Log Checkpoint v0\n9944\ndGhlIHZpZXcgZnJvbSB0aGUgdHJlZSB0b3BzIGlzIGdyZWF0IQ==\nHere's some associated data.",
			want: api.LogState{
				Size:     9944,
				RootHash: []byte("the view from the tree tops is great!"),
			},
		}, {
			desc:    "invalid header",
			m:       "Log Bananas v0\n9944\ndGhlIHZpZXcgZnJvbSB0aGUgdHJlZSB0b3BzIGlzIGdyZWF0IQ==\n",
			wantErr: true,
		}, {
			desc:    "invalid size - not a number",
			m:       "Log Checkpoint v0\nbananas\ndGhlIHZpZXcgZnJvbSB0aGUgdHJlZSB0b3BzIGlzIGdyZWF0IQ==\n",
			wantErr: true,
		}, {
			desc:    "invalid size - negative",
			m:       "Log Checkpoint v0\n-34\ndGhlIHZpZXcgZnJvbSB0aGUgdHJlZSB0b3BzIGlzIGdyZWF0IQ==\n",
			wantErr: true,
		}, {
			desc:    "invalid size - too large",
			m:       "Log Checkpoint v0\n3438945738945739845734895735\ndGhlIHZpZXcgZnJvbSB0aGUgdHJlZSB0b3BzIGlzIGdyZWF0IQ==\n",
			wantErr: true,
		}, {
			desc:    "invalid roothash - not base64",
			m:       "Log Checkpoint v0\n123\nThisIsn'tBase64\n",
			wantErr: true,
		},
	} {
		t.Run(string(test.desc), func(t *testing.T) {
			var got api.LogState
			if gotErr := got.UnmarshalText([]byte(test.m)); (gotErr != nil) != test.wantErr {
				t.Fatalf("UnmarshalText = %q, wantErr: %T", gotErr, test.wantErr)
			}
			if diff := cmp.Diff(test.want, got); len(diff) != 0 {
				t.Fatalf("UnmarshalText = diff %s", diff)
			}
		})
	}
}
