// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mysql

import (
	"context"
	"database/sql"
	"flag"
	"os"
	"testing"

	"github.com/golang/glog"
	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian-examples/gossip/incident/storage/mysql/testdb"

	_ "github.com/go-sql-driver/mysql" // Load MySQL driver
)

type entry struct {
	BaseURL, Summary string
	IsViolation      bool
	FullURL, Details string
}

func checkContents(ctx context.Context, testDB *sql.DB, t *testing.T, want []entry) {
	t.Helper()

	tx, err := testDB.BeginTx(ctx, nil /* opts */)
	if err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	defer tx.Commit()
	rows, err := tx.QueryContext(ctx, "SELECT BaseURL, Summary, IsViolation, FullURL, Details FROM Incidents;")
	if err != nil {
		t.Fatalf("failed to query rows: %v", err)
	}
	defer rows.Close()

	var got []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.BaseURL, &e.Summary, &e.IsViolation, &e.FullURL, &e.Details); err != nil {
			t.Fatalf("failed to scan row: %v", err)
		}
		got = append(got, e)
	}
	if err := rows.Err(); err != nil {
		t.Errorf("incident table iteration failed: %v", err)
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("incident table: diff (-got +want)\n%s", diff)
	}
}

func TestLogf(t *testing.T) {
	ctx := context.Background()
	testdb.Clean(ctx, testDB, "Incidents")

	checkContents(ctx, testDB, t, nil)

	reporter, err := NewMySQLReporter(ctx, testDB, "unittest")
	if err != nil {
		t.Fatalf("failed to build MySQLReporter: %v", err)
	}

	e := entry{BaseURL: "base", Summary: "summary", IsViolation: false, FullURL: "full", Details: "blah"}
	ev := entry{BaseURL: "base", Summary: "summary", IsViolation: true, FullURL: "full", Details: "blah"}

	reporter.LogUpdate(ctx, e.BaseURL, e.Summary, e.FullURL, e.Details)
	checkContents(ctx, testDB, t, []entry{e})

	reporter.LogViolation(ctx, ev.BaseURL, ev.Summary, ev.FullURL, ev.Details)
	checkContents(ctx, testDB, t, []entry{e, ev})

	reporter.LogUpdatef(ctx, e.BaseURL, e.Summary, e.FullURL, "%s", e.Details)
	checkContents(ctx, testDB, t, []entry{e, ev, e})
}

func TestMain(m *testing.M) {
	flag.Parse()
	if err := testdb.MySQLAvailable(); err != nil {
		glog.Errorf("MySQL not available, skipping all MySQL storage tests: %v", err)
		return
	}
	ctx := context.Background()
	var err error
	testDB, err = testdb.New(ctx, incidentSQL)
	if err != nil {
		glog.Exitf("failed to create test database: %v", err)
	}
	defer testDB.Close()
	testdb.Clean(ctx, testDB, "Incidents")
	ec := m.Run()
	os.Exit(ec)
}

var (
	testDB      *sql.DB
	incidentSQL = "incident.sql"
)
