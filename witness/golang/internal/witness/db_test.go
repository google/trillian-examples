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
	"context"
	"database/sql"
	"testing"

	"github.com/google/go-cmp/cmp"
	_ "github.com/mattn/go-sqlite3" // Load drivers for sqlite3
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestRoundTrip tests the happy paths (adding stuff and looking it up).
func TestRoundTrip(t *testing.T) {
	for _, test := range []struct {
		desc      string
		logID     string
		c         Chkpt
		extraRuns int
	}{
		{
			desc:  "simple",
			logID: "monkeys",
			c: Chkpt{
				Size: 123,
				Raw:  []byte("bananas"),
			},
		}, {
			desc:  "check no failure on inserting same data twice",
			logID: "monkeys",
			c: Chkpt{
				Size: 123,
				Raw:  []byte("bananas"),
			},
			extraRuns: 1,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ctx := context.Background()
			db, err := sql.Open("sqlite3", ":memory:")
			if err != nil {
				t.Fatalf("failed to open temporary in-memory DB: %v", err)
			}
			defer db.Close()

			d, err := NewDatabase(db)
			if err != nil {
				t.Fatalf("failed to create DB: %v", err)
			}

			prevChkpts := []uint64{0, test.c.Size}
			for i := 0; i <= test.extraRuns; i++ {
				if err := d.SetCheckpoint(ctx, test.logID, prevChkpts[i], test.c); err != nil {
					t.Errorf("failed to set checkpoint: %v", err)
				}
				got, err := d.GetLatest(test.logID)
				if err != nil {
					t.Errorf("failed to get latest: %v", err)
				}
				if diff := cmp.Diff(got, test.c); len(diff) != 0 {
					t.Errorf("latest checkpoint mismatch:\n%s", diff)
				}
			}
		})
	}
}

// TestUnknownKey should fail because nothing has been added.
func TestUnknownKey(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open temporary in-memory DB: %v", err)
	}
	defer db.Close()

	d, err := NewDatabase(db)
	if err != nil {
		t.Fatalf("failed to create DB: %v", err)
	}

	logID := "monkeys"

	r, err := d.GetLatest(logID)
	if err == nil {
		t.Fatalf("want error, but got none, result: %v", r)
	}
	if got, want := status.Code(err), codes.NotFound; got != want {
		t.Fatalf("got error code %s, want %s", got, want)
	}
}

// TestOutdatedChkpt should fail because caller's latest checkpoint is out of
// date.
func TestOutdatedChkpt(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open temporary in-memory DB: %v", err)
	}
	defer db.Close()

	d, err := NewDatabase(db)
	if err != nil {
		t.Fatalf("failed to create DB: %v", err)
	}

	logID := "monkeys"

	c1 := Chkpt{
		Size: 1,
		Raw:  []byte("bananas"),
	}
	c2 := Chkpt{
		Size: 2,
		Raw:  []byte("bananas"),
	}
	c3 := Chkpt{
		Size: 3,
		Raw:  []byte("bananas"),
	}

	if err := d.SetCheckpoint(ctx, logID, 0, c1); err != nil {
		t.Errorf("failed to set checkpoint: %v", err)
	}
	if err := d.SetCheckpoint(ctx, logID, c1.Size, c2); err != nil {
		t.Errorf("failed to set checkpoint: %v", err)
	}
	if err := d.SetCheckpoint(ctx, logID, c1.Size, c3); err == nil {
		t.Fatalf("want error, but got none")
	}
}

// TestNilChkpt should fail because caller is "lying" about there being no
// checkpoint stored for this key.
func TestNilChkpt(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open temporary in-memory DB: %v", err)
	}
	defer db.Close()

	d, err := NewDatabase(db)
	if err != nil {
		t.Fatalf("failed to create DB: %v", err)
	}

	logID := "monkeys"

	c1 := Chkpt{
		Size: 1,
		Raw:  []byte("bananas"),
	}
	c2 := Chkpt{
		Size: 2,
		Raw:  []byte("bananas"),
	}

	if err := d.SetCheckpoint(ctx, logID, 0, c1); err != nil {
		t.Errorf("failed to set checkpoint: %v", err)
	}
	if err := d.SetCheckpoint(ctx, logID, 0, c2); err == nil {
		t.Fatalf("want error, but got none")
	}
}
