// Copyright 2022 Google LLC. All Rights Reserved.
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

package sql

import (
	"io/ioutil"
	"os"
	"testing"

	"database/sql"

	"github.com/google/trillian-examples/witness/golang/internal/persistence"
	ptest "github.com/google/trillian-examples/witness/golang/internal/persistence/testonly"
	_ "github.com/mattn/go-sqlite3" // Load drivers for sqlite3
)

func TestGetLogs(t *testing.T) {
	ptest.TestGetLogs(t, func() (persistence.LogStatePersistence, func() error) {
		db, close := mustCreateDB(t)
		return NewSqlPersistence(db), close
	})
}

func TestWriteOps(t *testing.T) {
	ptest.TestWriteOps(t, func() (persistence.LogStatePersistence, func() error) {
		db, close := mustCreateDB(t)
		return NewSqlPersistence(db), close
	})
}

func mustCreateDB(t *testing.T) (*sql.DB, func() error) {
	t.Helper()
	// Use a file to try to get as close to real sqlite behaviour as possible.
	// Concurrent transactions still don't really work though.
	f, err := ioutil.TempFile("", "dbtest*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	db, err := sql.Open("sqlite3", f.Name())
	if err != nil {
		t.Fatalf("failed to open temporary DB: %v", err)
	}
	db.SetMaxOpenConns(1)
	return db, func() error {
		defer os.Remove(f.Name())
		return db.Close()
	}
}
