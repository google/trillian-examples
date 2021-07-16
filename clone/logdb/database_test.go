// Copyright 2020 Google LLC. All Rights Reserved.
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

package logdb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	_ "github.com/mattn/go-sqlite3" // Load drivers for sqlite3
)

func TestHeadIncremented(t *testing.T) {
	for _, test := range []struct {
		desc    string
		leaves  [][]byte
		want    int64
		wantErr error
	}{
		{
			desc:    "no data",
			wantErr: ErrNoDataFound,
		}, {
			desc:   "one leaf",
			leaves: [][]byte{[]byte("first!")},
			want:   0,
		}, {
			desc:   "many leaves",
			leaves: [][]byte{[]byte("a"), []byte("b"), []byte("c")},
			want:   2,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			db, close, err := NewInMemoryDatabase()
			if err != nil {
				t.Fatal("failed to init DB", err)
			}
			defer close()
			if err := db.WriteLeaves(context.Background(), 0, test.leaves); err != nil {
				t.Fatal("failed to write leaves", err)
			}

			head, err := db.Head()
			if test.wantErr != err {
				t.Errorf("expected err %q but got %q", test.wantErr, err)
			}
			if test.wantErr != nil {
				if head != test.want {
					t.Errorf("expected %d but got %d", test.want, head)
				}
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	leaves := [][]byte{
		[]byte("aa"),
		[]byte("bb"),
		[]byte("cc"),
		[]byte("dd"),
	}
	for _, test := range []struct {
		desc       string
		leaves     [][]byte
		start, end uint64
		wantLeaves [][]byte
	}{{
		desc:       "one leaf",
		leaves:     leaves[:1],
		start:      0,
		end:        1,
		wantLeaves: leaves[:1],
	}, {
		desc:       "many leaves pick all",
		leaves:     leaves,
		start:      0,
		end:        uint64(len(leaves)),
		wantLeaves: leaves,
	}, {
		desc:       "many leaves select middle",
		leaves:     leaves,
		start:      1,
		end:        3,
		wantLeaves: leaves[1:3],
	}, {
		desc:       "many leaves start past the end",
		leaves:     leaves,
		start:      100,
		wantLeaves: [][]byte{},
	},
	} {
		t.Run(test.desc, func(t *testing.T) {
			db, close, err := NewInMemoryDatabase()
			if err != nil {
				t.Fatal("failed to init DB", err)
			}
			defer close()
			if err := db.WriteLeaves(context.Background(), 0, test.leaves); err != nil {
				t.Fatal("failed to write leaves", err)
			}

			lc := make(chan []byte, 1)
			errc := make(chan error)
			go db.StreamLeaves(test.start, test.end, lc, errc)

			got := make([][]byte, 0)
		Receive:
			for l := range lc {
				select {
				case err = <-errc:
					break Receive
				default:
				}
				got = append(got, l)
			}

			if err != nil {
				t.Fatalf("unexpected error: %q", err)
			}

			if diff := cmp.Diff(got, test.wantLeaves); len(diff) > 0 {
				t.Errorf("diff in leaves: %q", diff)
			}
		})
	}
}

func TestCheckpointRoundTrip(t *testing.T) {
	hashes := make([][]byte, 5)
	for i := range hashes {
		h := sha256.Sum256([]byte(fmt.Sprintf("hash %d", i)))
		hashes[i] = h[:]
	}
	for _, test := range []struct {
		desc         string
		checkpoint   []byte
		compactRange [][]byte
		size         uint64
		err          error
	}{{
		desc: "no previous",
		err:  ErrNoDataFound,
	}, {
		desc:         "single compact range",
		size:         256,
		checkpoint:   hashes[0],
		compactRange: hashes[1:2],
	}, {
		desc:         "longer compact range",
		size:         277,
		checkpoint:   hashes[0],
		compactRange: hashes[1:],
	},
	} {
		t.Run(test.desc, func(t *testing.T) {
			db, close, err := NewInMemoryDatabase()
			if err != nil {
				t.Fatal("failed to init DB", err)
			}
			defer close()
			if test.checkpoint != nil {
				if err := db.WriteCheckpoint(context.Background(), test.size, test.checkpoint, test.compactRange); err != nil {
					t.Fatal("failed to write checkpoint", err)
				}
			}

			gotSize, gotCP, gotCR, gotErr := db.GetLatestCheckpoint(context.Background())

			if gotErr != test.err {
				t.Fatalf("mismatched error: got=%v, want %v", gotErr, test.err)
			}

			if gotSize != test.size {
				t.Errorf("size: got %d, want %d", gotSize, test.size)
			}
			if !bytes.Equal(gotCP, test.checkpoint) {
				t.Errorf("checkpoint: got %x, want %x", gotCP, test.checkpoint)
			}
			if !reflect.DeepEqual(gotCR, test.compactRange) {
				t.Errorf("compact range: got != want: \n%v\n%v", gotCR, test.compactRange)
			}
		})
	}
}

func TestCheckpoints(t *testing.T) {
	for _, test := range []struct {
		desc          string
		size1         uint64
		checkpoint1   []byte
		compactRange1 [][]byte
		size2         uint64
		checkpoint2   []byte
		compactRange2 [][]byte
		wantSize      uint64
		wantCP        []byte
	}{{
		desc:          "small, big",
		size1:         16,
		checkpoint1:   mustHash("root 1"),
		compactRange1: [][]byte{mustHash("root 1")},
		size2:         32,
		checkpoint2:   mustHash("root 2"),
		compactRange2: [][]byte{mustHash("root 2")},
		wantSize:      32,
		wantCP:        mustHash("root 2"),
	}, {
		desc:          "big, small",
		size1:         32,
		checkpoint1:   mustHash("root 1"),
		compactRange1: [][]byte{mustHash("root 1")},
		size2:         16,
		checkpoint2:   mustHash("root 2"),
		compactRange2: [][]byte{mustHash("root 2")},
		wantSize:      32,
		wantCP:        mustHash("root 1"),
	}, {
		desc:          "same checkpoint twice",
		size1:         16,
		checkpoint1:   mustHash("root 1"),
		compactRange1: [][]byte{mustHash("root 1")},
		size2:         16,
		checkpoint2:   mustHash("root 1"),
		compactRange2: [][]byte{mustHash("root 1")},
		wantSize:      16,
		wantCP:        mustHash("root 1"),
	}, {
		desc:          "unequal checkpoints for same size",
		size1:         16,
		checkpoint1:   mustHash("root 1"),
		compactRange1: [][]byte{mustHash("root 1")},
		size2:         16,
		checkpoint2:   mustHash("root 2"),
		compactRange2: [][]byte{mustHash("root 2")},
		wantSize:      16,
		wantCP:        mustHash("root 1"),
	},
	} {
		t.Run(test.desc, func(t *testing.T) {
			db, close, err := NewInMemoryDatabase()
			if err != nil {
				t.Fatal("failed to init DB", err)
			}
			defer close()

			if err := db.WriteCheckpoint(context.Background(), test.size1, test.checkpoint1, test.compactRange1); err != nil {
				t.Fatal("failed to write checkpoint 1", err)
			}
			if err := db.WriteCheckpoint(context.Background(), test.size2, test.checkpoint2, test.compactRange2); err != nil {
				t.Fatal("failed to write checkpoint 2", err)
			}

			gotSize, cp, _, err := db.GetLatestCheckpoint(context.Background())
			if err != nil {
				t.Fatalf("GetLatestCheckpoint(): %v", err)
			}

			if gotSize != test.wantSize {
				t.Errorf("size: got %d, want %d", gotSize, test.wantSize)
			}
			if !bytes.Equal(cp, test.wantCP) {
				t.Errorf("checkpoint: got %x, want %x", cp, test.wantCP)
			}
		})
	}
}

func NewInMemoryDatabase() (*Database, func() error, error) {
	sqlitedb, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open temporary in-memory DB: %v", err)
	}
	db, err := NewDatabaseDirect(sqlitedb)
	return db, sqlitedb.Close, err
}

func mustHash(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}
