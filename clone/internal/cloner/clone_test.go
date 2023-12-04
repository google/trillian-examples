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

// clone contains the core engine for quickly downloading leaves and adding
// them to the database.
package cloner

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/google/trillian-examples/clone/logdb"
	_ "github.com/mattn/go-sqlite3" // Load drivers for sqlite3
)

func TestClone(t *testing.T) {
	for _, test := range []struct {
		name                                    string
		first, treeSize                         uint64
		workers, fetchBatchSize, writeBatchSize uint
	}{
		{
			name:           "smallest batch",
			first:          0,
			treeSize:       10,
			workers:        1,
			fetchBatchSize: 1,
			writeBatchSize: 1,
		},
		{
			name:           "normal settings",
			first:          0,
			treeSize:       100,
			workers:        4,
			fetchBatchSize: 5,
			writeBatchSize: 10,
		},
		{
			name:           "oversized batches",
			first:          0,
			treeSize:       10,
			workers:        10,
			fetchBatchSize: 100,
			writeBatchSize: 100,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, close, err := newInMemoryDatabase()
			if err != nil {
				t.Fatalf("NewDatabase(): %v", err)
			}
			defer close()

			cloner := New(test.workers, test.fetchBatchSize, test.writeBatchSize, db)
			fetcher := func(start uint64, leaves [][]byte) error {
				for i := range leaves {
					leaves[i] = []byte(fmt.Sprintf("leaf %d", start+1))
				}
				return nil
			}
			if err := cloner.clone(context.Background(), test.treeSize, fetcher); err != nil {
				t.Fatalf("Clone(): %v", err)
			}

			if head, err := db.Head(); err != nil {
				t.Fatalf("Head(): %v", err)
			} else if gotSize := head + 1; gotSize != int64(test.treeSize) {
				t.Errorf("got tree size %d, want %d", gotSize, test.treeSize)
			}
		})
	}
}

func newInMemoryDatabase() (*logdb.Database, func(), error) {
	sqlitedb, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open temporary in-memory DB: %v", err)
	}
	db, err := logdb.NewDatabaseDirect(sqlitedb)
	return db, func() { _ = sqlitedb.Close }, err
}
