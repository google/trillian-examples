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
	"context"
	"database/sql"
	"testing"

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
			sqlitedb, err := sql.Open("sqlite3", ":memory:")
			if err != nil {
				t.Fatal("failed to open temporary in-memory DB", err)
			}
			defer sqlitedb.Close()

			db := Database{sqlitedb}
			if err := db.Init(); err != nil {
				t.Fatal("failed to init DB", err)
			}
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
