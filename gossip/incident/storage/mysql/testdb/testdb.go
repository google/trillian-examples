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

// testdb contains helper functions for testing mysql storage
package testdb

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/golang/glog"

	_ "github.com/go-sql-driver/mysql" // Load MySQL driver
)

var (
	dataSource = "root@tcp(127.0.0.1)/"
)

// MySQLAvailable indicates whether a default MySQL database is available.
func MySQLAvailable() error {
	db, err := sql.Open("mysql", dataSource)
	if err != nil {
		return fmt.Errorf("sql.Open(): %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		return fmt.Errorf("db.Ping(): %v", err)
	}
	return nil
}

// newEmptyDB creates a new, empty database.
func newEmptyDB(ctx context.Context) (*sql.DB, error) {
	db, err := sql.Open("mysql", dataSource)
	if err != nil {
		return nil, err
	}

	// Create a randomly-named database and then connect using the new name.
	name := fmt.Sprintf("mono_%v", time.Now().UnixNano())

	stmt := fmt.Sprintf("CREATE DATABASE %v", name)
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return nil, fmt.Errorf("error running statement %q: %v", stmt, err)
	}
	db.Close()

	db, err = sql.Open("mysql", dataSource+name+"?parseTime=true")
	if err != nil {
		return nil, fmt.Errorf("failed to open new database %q: %v", name, err)
	}
	return db, db.Ping()
}

// NewDB creates an empty database with the given schema.
func New(ctx context.Context, schemaPath string) (*sql.DB, error) {
	db, err := newEmptyDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create empty DB: %v", err)
	}

	sqlBytes, err := ioutil.ReadFile(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema SQL: %v", err)
	}

	for _, stmt := range strings.Split(sanitize(string(sqlBytes)), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return nil, fmt.Errorf("error running statement %q: %v", stmt, err)
		}
	}
	return db, nil
}

func sanitize(script string) string {
	buf := &bytes.Buffer{}
	for _, line := range strings.Split(string(script), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' || strings.Index(line, "--") == 0 {
			continue // skip empty lines and comments
		}
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	return buf.String()
}

func Clean(ctx context.Context, testDB *sql.DB, name string) {
	if _, err := testDB.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s", name)); err != nil {
		glog.Exitf("Failed to delete rows in %s: %v", name, err)
	}
}
