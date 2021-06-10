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

package witness

import (
	"bytes"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3" // Load drivers for sqlite3
)

func TestRoundTrip(t *testing.T) {
	for _, test := range []struct {
		desc      string
		logPK     string
		c         Chkpt
		extraRuns int
	}{
		{
			desc:  "simple",
			logPK: "monkeys",
			c: Chkpt{
				Size: 123,
				Raw:  []byte("bananas"),
			},
		}, {
			desc:  "check no failure on inserting same data twice",
			logPK: "monkeys",
			c: Chkpt{
				Size: 123,
				Raw:  []byte("bananas"),
			},
			extraRuns: 1,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			db, err := sql.Open("sqlite3", ":memory:")
			if err != nil {
				t.Error("failed to open temporary in-memory DB", err)
			}
			defer db.Close()

			d, err := NewDatabase(db)
			if err != nil {
				t.Error("failed to create DB", err)
			}

			for i := 0; i < test.extraRuns+1; i++ {
				if err := d.SetCheckpoint(test.logPK, test.c); err != nil {
					t.Error("failed to set checkpoint", err)
				}
				got, err := d.GetLatest(test.logPK)
				if err != nil {
					t.Error("failed to get latest", err)
				}
				if got.Size != test.c.Size || !bytes.Equal(got.Raw, test.c.Raw) {
					t.Errorf("got != want (%x, %x)", got.Size, test.c.Size)
				}
			}
		})
	}
}

func TestUnknownHash(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Error("failed to open temporary in-memory DB", err)
	}
	defer db.Close()

	d, err := NewDatabase(db)
	if err != nil {
		t.Error("failed to create DB", err)
	}

	logPK := "monkeys"

	r, err := d.GetLatest(logPK)
	if err == nil {
		t.Fatalf("want error, but got none, result: %v", r)
	}
}
