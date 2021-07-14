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

const (
	// This can be changed to a field of Database or metadata in the database if
	// we need to support other sizes in the future.
	hashSizeBytes = 32
)

// ErrNoDataFound is returned when the DB appears valid but has no data in it.
var ErrNoDataFound = errors.New("no data found")

// Database provides read/write access to the mirrored log.
type Database struct {
	db *sql.DB
}

// NewDatabase creates a Database using the given database connection string.
// This has been tested with sqlite and MariaDB.
func NewDatabase(connString string) (*Database, error) {
	dbConn, err := sql.Open("mysql", connString)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	db := &Database{
		db: dbConn,
	}
	return db, db.init()
}

// NewDatabase creates a Database using the given database connection.
func NewDatabaseDirect(db *sql.DB) (*Database, error) {
	ret := &Database{
		db: db,
	}
	return ret, ret.init()
}

func (d *Database) init() error {
	if _, err := d.db.Exec("CREATE TABLE IF NOT EXISTS leaves (id INTEGER PRIMARY KEY, data BLOB)"); err != nil {
		return err
	}
	if _, err := d.db.Exec("CREATE TABLE IF NOT EXISTS checkpoints (size INTEGER PRIMARY KEY, data BLOB, compactRange BLOB)"); err != nil {
		return err
	}
	return nil
}

// WriteCheckpoint writes the checkpoint for the given tree size.
// This should have been verified before writing.
func (d *Database) WriteCheckpoint(ctx context.Context, size uint64, checkpoint []byte, compactRange [][]byte) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("BeginTx(): %v", err)
	}

	row := tx.QueryRowContext(ctx, "SELECT size FROM checkpoints ORDER BY size DESC LIMIT 1")
	var max uint64
	if err := row.Scan(&max); err != nil {
		if err != sql.ErrNoRows {
			tx.Rollback()
			return fmt.Errorf("Scan(): %v", err)
		}
	}

	if size <= max {
		tx.Rollback()
		return nil
	}

	srs := make([]byte, hashSizeBytes*len(compactRange))
	for i, cr := range compactRange {
		from := i * hashSizeBytes
		to := from + hashSizeBytes
		copy(srs[from:to], cr)
	}
	tx.ExecContext(ctx, "INSERT INTO checkpoints (size, data, compactRange) VALUES (?, ?, ?)", size, checkpoint, srs)
	return tx.Commit()
}

// GetLatestCheckpoint gets the details of the latest checkpoint.
func (d *Database) GetLatestCheckpoint(ctx context.Context) (size uint64, checkpoint []byte, compactRange [][]byte, err error) {
	row := d.db.QueryRowContext(ctx, "SELECT size, data, compactRange FROM checkpoints ORDER BY size DESC LIMIT 1")
	srs := make([]byte, 0)
	if err := row.Scan(&size, &checkpoint, &srs); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil, nil, ErrNoDataFound
		}
		return 0, nil, nil, fmt.Errorf("Scan(): %v", err)
	}
	compactRange = make([][]byte, len(srs)/hashSizeBytes)
	for i := range compactRange {
		from := i * hashSizeBytes
		to := from + hashSizeBytes
		compactRange[i] = srs[from:to]
	}
	return
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

// StreamLeaves streams leaves in order starting at the given index, putting the leaf preimage
// values on the `out` channel.
func (d *Database) StreamLeaves(start, end uint64, out chan []byte, errc chan error) {
	defer close(out)
	rows, err := d.db.Query("SELECT data FROM leaves WHERE id>=? AND id < ? ORDER BY id", start, end)
	if err != nil {
		errc <- err
		return
	}
	defer rows.Close()
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			errc <- err
			return
		}
		out <- data
	}
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
