// Copyright 2020 Google LLC
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

package audit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// NoDataFound is returned when the DB appears valid but has no data in it.
type NoDataFound = error

// Metadata is the semantic data that is contained within the leaves of the log.
type Metadata struct {
	module, version, repoHash, modHash string
}

// Database provides read/write access to the local copy of the SumDB.
type Database struct {
	db *sql.DB
}

// NewDatabase creates a Database using a file at the given location.
// If the file doesn't exist it will be created.
func NewDatabase(location string) (*Database, error) {
	db, err := sql.Open("sqlite3", location)
	if err != nil {
		return nil, err
	}
	return &Database{
		db: db,
	}, nil
}

// Init creates the database tables if needed.
func (d *Database) Init() error {
	if _, err := d.db.Exec("CREATE TABLE IF NOT EXISTS leaves (id INTEGER PRIMARY KEY, data BLOB)"); err != nil {
		return err
	}
	if _, err := d.db.Exec("CREATE TABLE IF NOT EXISTS tiles (height INTEGER, level INTEGER, offset INTEGER, hashes BLOB, PRIMARY KEY (height, level, offset))"); err != nil {
		return err
	}
	if _, err := d.db.Exec("CREATE TABLE IF NOT EXISTS checkpoints (datetime TIMESTAMP PRIMARY KEY, checkpoint BLOB)"); err != nil {
		return err
	}
	_, err := d.db.Exec("CREATE TABLE IF NOT EXISTS leafMetadata (id INTEGER PRIMARY KEY, module TEXT, version TEXT, repohash TEXT, modhash TEXT)")
	return err
}

// Head returns the largest leaf index written.
func (d *Database) Head() (int64, error) {
	var head sql.NullInt64
	if err := d.db.QueryRow("SELECT MAX(id) AS head FROM leaves").Scan(&head); err != nil {
		return 0, fmt.Errorf("failed to get max revision: %v", err)
	}
	if head.Valid {
		return head.Int64, nil
	}
	return 0, NoDataFound(errors.New("no data found"))
}

// GoldenCheckpoint gets the latest checkpoint, using the provided function to parse the note data.
func (d *Database) GoldenCheckpoint(parse func([]byte) (*Checkpoint, error)) (*Checkpoint, error) {
	var datetime sql.NullTime
	var data []byte
	if err := d.db.QueryRow("SELECT datetime, checkpoint FROM checkpoints ORDER BY datetime DESC LIMIT 1").Scan(&datetime, &data); err != nil {
		return nil, fmt.Errorf("failed to get max revision: %v", err)
	}
	if !datetime.Valid {
		return nil, NoDataFound(errors.New("no data found"))
	}
	return parse(data)
}

// SetGoldenCheckpoint records the given checkpoint to the database.
func (d *Database) SetGoldenCheckpoint(cp *Checkpoint) error {
	now := time.Now()
	_, err := d.db.Exec("INSERT INTO checkpoints (datetime, checkpoint) VALUES (?, ?)", now, cp.Raw)
	if err != nil {
		return fmt.Errorf("failed to insert checkpoint: %w", err)
	}
	return nil
}

// WriteLeaves writes the contiguous chunk of leaves, starting at the stated index.
// This is an atomic operation, and will fail if any leaf cannot be inserted.
func (d *Database) WriteLeaves(ctx context.Context, start int64, leaves [][]byte) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("BeginTx: %v", err)
	}
	for li, l := range leaves {
		lidx := int64(li) + start
		tx.Exec("INSERT INTO leaves (id, data) VALUES (?, ?)", lidx, l)
	}
	return tx.Commit()
}

// Leaves gets a contiguous block of leaves.
func (d *Database) Leaves(start int64, count int) ([][]byte, error) {
	var res [][]byte
	rows, err := d.db.Query("SELECT data FROM leaves WHERE id>=? AND id<? ORDER BY id", start, start+int64(count))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var data []byte
		rows.Scan(&data)
		res = append(res, data)
	}
	if len(res) != count {
		return nil, fmt.Errorf("failed to read %d leaves, only found %d", count, len(res))
	}
	return res, err
}

// SetLeafMetadata sets the metadata for a contiguous batch of leaves.
// This is an atomic operation, and will fail if any metadata cannot be inserted.
func (d *Database) SetLeafMetadata(ctx context.Context, start int64, metadata []Metadata) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("BeginTx: %v", err)
	}
	for mi, m := range metadata {
		midx := int64(mi) + start
		tx.Exec("INSERT INTO leafMetadata (id, module, version, repohash, modhash) VALUES (?, ?, ?, ?, ?)", midx, m.module, m.version, m.repoHash, m.modHash)
	}
	return tx.Commit()
}

// Tile gets the leaf hashes for the given tile, or returns an error.
func (d *Database) Tile(height, level, offset int) ([][]byte, error) {
	var res []byte
	err := d.db.QueryRow("SELECT hashes FROM tiles WHERE height=? AND level=? AND offset=?", height, level, offset).Scan(&res)
	if err != nil {
		return nil, err
	}
	return SplitTile(res, height), nil
}

// SetTile sets the leaf hash data for the given tile.
// The leaf hashes should be 2^height * HashLenBytes long.
func (d *Database) SetTile(height, level, offset int, hashes []byte) error {
	if got, want := len(hashes), (1<<height)*HashLenBytes; got != want {
		return fmt.Errorf("wanted %d tile hash bytes but got %d", want, got)
	}
	_, err := d.db.Exec("INSERT INTO tiles (height, level, offset, hashes) VALUES (?, ?, ?, ?)", height, level, offset, hashes)
	return err
}

// SplitTile turns the blob that is the leaf hashes in a tile into separate hashes.
func SplitTile(hashes []byte, height int) [][]byte {
	tileWidth := 1 << height
	res := make([][]byte, tileWidth)
	for i := 0; i < tileWidth; i++ {
		hash := hashes[i*HashLenBytes : (i+1)*HashLenBytes]
		res[i] = hash
	}
	return res
}
