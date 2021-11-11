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

package verify

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/google/trillian-examples/clone/logdb"
	_ "github.com/mattn/go-sqlite3" // Load drivers for sqlite3
	"github.com/transparency-dev/merkle/rfc6962"
)

func TestRootFromScratch(t *testing.T) {
	db, close, err := NewInMemoryDatabase()
	if err != nil {
		t.Fatalf("NewDatabase(): %v", err)
	}
	defer close()

	leaves := make([][]byte, 64)
	for i := range leaves {
		leaves[i] = []byte(fmt.Sprintf("leaf %d", i))
	}
	if err := db.WriteLeaves(context.Background(), 0, leaves); err != nil {
		t.Fatalf("Failed to initialize database with leaves")
	}

	h := rfc6962.DefaultHasher
	lh := func(_ uint64, preimage []byte) []byte {
		return h.HashLeaf(preimage)
	}

	for _, test := range []struct {
		desc     string
		count    uint64
		wantRoot string
		wantErr  bool
	}{{
		desc:     "one leaf",
		count:    1,
		wantRoot: "G7l9zCFjXUfiZj79/QoXRobZjdcBNS3SzQbotD/T0wU",
	}, {
		desc:     "16 leaves",
		count:    16,
		wantRoot: "cUopKYyn2GQ5dRAFaUmcIwnoOm8vlwkC3EbvMuvBsA8",
	}, {
		desc:     "17 leaves",
		count:    17,
		wantRoot: "Ru8bykxkgM1l5Q4pzBw3XbNnEc1QJF7NPmsxDG4qOD8",
	}, {
		desc:     "all leaves",
		count:    64,
		wantRoot: "8mHiNpLZeP2sP9lJ21SVlApDeuZxuabd6aphGNADZS8",
	}, {
		desc:    "too many leaves",
		count:   65,
		wantErr: true,
	},
	} {
		t.Run(test.desc, func(t *testing.T) {
			v := NewLogVerifier(db, lh, h.HashChildren)
			got, _, err := v.MerkleRoot(context.Background(), test.count)
			if gotErr := err != nil; test.wantErr != gotErr {
				t.Errorf("expected err (%t) but got: %q", test.wantErr, err)
			}
			if !test.wantErr {
				gotb64 := base64.RawStdEncoding.EncodeToString(got)
				if gotb64 != test.wantRoot {
					t.Errorf("got %q but wanted root %q", gotb64, test.wantRoot)
				}
			}
		})
	}
}

func TestPartialRoot(t *testing.T) {
	db, close, err := NewInMemoryDatabase()
	if err != nil {
		t.Fatalf("NewDatabase(): %v", err)
	}
	defer close()

	leaves := make([][]byte, 64)
	for i := range leaves {
		leaves[i] = []byte(fmt.Sprintf("leaf %d", i))
	}
	if err := db.WriteLeaves(context.Background(), 0, leaves); err != nil {
		t.Fatalf("Failed to initialize database with leaves: %v", err)
	}
	cr, err := base64.RawStdEncoding.DecodeString("cUopKYyn2GQ5dRAFaUmcIwnoOm8vlwkC3EbvMuvBsA8")
	if err != nil {
		t.Fatalf("Failed to decode base64: %v", err)
	}
	if err := db.WriteCheckpoint(context.Background(), 16, []byte("root"), [][]byte{cr}); err != nil {
		t.Fatalf("Failed to init db with checkpoint: %v", err)
	}

	h := rfc6962.DefaultHasher
	lh := func(_ uint64, preimage []byte) []byte {
		return h.HashLeaf(preimage)
	}

	for _, test := range []struct {
		desc     string
		count    uint64
		wantRoot string
		wantErr  bool
	}{{
		desc:     "one leaf",
		count:    1,
		wantRoot: "G7l9zCFjXUfiZj79/QoXRobZjdcBNS3SzQbotD/T0wU",
	}, {
		desc:     "16 leaves",
		count:    16,
		wantRoot: "cUopKYyn2GQ5dRAFaUmcIwnoOm8vlwkC3EbvMuvBsA8",
	}, {
		desc:     "17 leaves",
		count:    17,
		wantRoot: "Ru8bykxkgM1l5Q4pzBw3XbNnEc1QJF7NPmsxDG4qOD8",
	}, {
		desc:     "all leaves",
		count:    64,
		wantRoot: "8mHiNpLZeP2sP9lJ21SVlApDeuZxuabd6aphGNADZS8",
	}, {
		desc:    "too many leaves",
		count:   65,
		wantErr: true,
	},
	} {
		t.Run(test.desc, func(t *testing.T) {
			v := NewLogVerifier(db, lh, h.HashChildren)
			got, _, err := v.MerkleRoot(context.Background(), test.count)
			if gotErr := err != nil; test.wantErr != gotErr {
				t.Errorf("expected err (%t) but got: %q", test.wantErr, err)
			}
			if !test.wantErr {
				gotb64 := base64.RawStdEncoding.EncodeToString(got)
				if gotb64 != test.wantRoot {
					t.Errorf("got %q but wanted root %q", gotb64, test.wantRoot)
				}
			}
		})
	}
}

func NewInMemoryDatabase() (*logdb.Database, func() error, error) {
	sqlitedb, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open temporary in-memory DB: %v", err)
	}
	db, err := logdb.NewDatabaseDirect(sqlitedb)
	return db, sqlitedb.Close, err
}
