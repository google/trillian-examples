// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// logdb contains read/write access to the locally cloned data.
package logdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrNoDataFound is returned when the DB appears valid but has no data in it.
var ErrNoDataFound = errors.New("no data found")

// Database provides read/write access to the mirrored log.
type Database struct {
	db *sql.DB
}

// NewDatabase creates a Database using the given database connection.
// This has been tested with sqlite and MariaDB.
func NewDatabase(connString string) (*Database, error) {
	dbConn, err := sql.Open("mysql", connString)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	db := &Database{
		db: dbConn,
	}
	return db, db.Init()
}

// Init creates the database tables if needed.
func (d *Database) Init() error {
	if _, err := d.db.Exec("CREATE TABLE IF NOT EXISTS leaves (id INTEGER PRIMARY KEY, data BLOB)"); err != nil {
		return err
	}
	// TODO(mhutchinson): Create a table for checkpoints too?
	return nil
}

// WriteLeaves writes the contiguous chunk of leaves, starting at the stated index.
// This is an atomic operation, and will fail if any leaf cannot be inserted.
func (d *Database) WriteLeaves(ctx context.Context, start uint64, leaves [][]byte) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("BeginTx: %w", err)
	}
	for li, l := range leaves {
		lidx := uint64(li) + start
		tx.Exec("INSERT INTO leaves (id, data) VALUES (?, ?)", lidx, l)
	}
	return tx.Commit()
}

// Leaves returns `count` leaves starting from index `start`.
func (d *Database) Leaves(start uint64, count uint) ([][]byte, error) {
	var res [][]byte
	rows, err := d.db.Query("SELECT data FROM leaves WHERE id>=? AND id<? ORDER BY id", start, start+uint64(count))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var data []byte
		rows.Scan(&data)
		res = append(res, data)
	}
	if len(res) != int(count) {
		return nil, fmt.Errorf("failed to read %d leaves, only found %d", count, len(res))
	}
	return res, err
}

// Head returns the largest leaf index written.
func (d *Database) Head() (int64, error) {
	var head sql.NullInt64
	if err := d.db.QueryRow("SELECT MAX(id) AS head FROM leaves").Scan(&head); err != nil {
		if err == sql.ErrNoRows {
			return 0, ErrNoDataFound
		}
		return 0, fmt.Errorf("failed to get max revision: %w", err)
	}
	if head.Valid {
		return head.Int64, nil
	}
	return 0, ErrNoDataFound
}
