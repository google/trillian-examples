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
	"flag"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/sumdbaudit/client"

	// While flags are defined in this file, the drivers can be imported here.
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

// hashLenBytes is the number of bytes in the SumDB hashes.
const hashLenBytes = 32

var (
	sqliteFile = flag.String("sqlite_file", "", "database file location for local SumDB instance")
	mysqlURI   = flag.String("mysql_uri", "", "URL of a MySQL database for the local SumDB instance")
)

// ErrNoDataFound is returned when the DB appears valid but has no data in it.
var ErrNoDataFound = errors.New("no data found")

// Metadata is the semantic data that is contained within the leaves of the log.
type Metadata struct {
	module, version, repoHash, modHash string
}

// Duplicate is used to report any number of module+version entries appearing more
// than once.
type Duplicate struct {
	Module  string
	Version string
	Count   int
}

// Database provides read/write access to the local copy of the SumDB.
type Database struct {
	db *sql.DB
}

// NewDatabaseFromFlags creates a database from the flags defined in this file.
// TODO(mhutchinson): This feels ugly to define flags not in the main method.
func NewDatabaseFromFlags() (*Database, error) {
	useSqlite := len(*sqliteFile) > 0
	useMysql := len(*mysqlURI) > 0
	if useSqlite == useMysql {
		return nil, errors.New("exactly one of sqlite_file or mysql_uri must be provided")
	}
	var dbConn *sql.DB
	var err error
	if useSqlite {
		dbConn, err = sql.Open("sqlite3", *sqliteFile)
	} else {
		// An alternative to providing a single URI is to have flags for individual components and
		// assemble the URI: https://godoc.org/github.com/go-sql-driver/mysql#Config.FormatDSN
		dbConn, err = sql.Open("mysql", *mysqlURI)
	}
	if err != nil {
		glog.Exitf("Failed to open DB: %v", err)
	}
	return NewDatabase(dbConn)
}

// NewDatabase creates a Database using the given database connection.
// This has been tested with sqlite and MariaDB.
func NewDatabase(db *sql.DB) (*Database, error) {
	return &Database{
		db: db,
	}, db.Ping()
}

// Init creates the database tables if needed.
func (d *Database) Init() error {
	if _, err := d.db.Exec("CREATE TABLE IF NOT EXISTS leaves (id INTEGER PRIMARY KEY, data BLOB)"); err != nil {
		return err
	}
	if _, err := d.db.Exec("CREATE TABLE IF NOT EXISTS tiles (height INTEGER, level INTEGER, toffset INTEGER, hashes BLOB, PRIMARY KEY (height, level, toffset))"); err != nil {
		return err
	}
	if _, err := d.db.Exec("CREATE TABLE IF NOT EXISTS checkpoints (datetime TIMESTAMP PRIMARY KEY, checkpoint BLOB)"); err != nil {
		return err
	}
	_, err := d.db.Exec("CREATE TABLE IF NOT EXISTS leafMetadata (id INTEGER PRIMARY KEY, module BLOB, version BLOB, repohash BLOB, modhash BLOB)")
	return err
}

// Head returns the largest leaf index written.
func (d *Database) Head() (int64, error) {
	var head sql.NullInt64
	if err := d.db.QueryRow("SELECT MAX(id) AS head FROM leaves").Scan(&head); err != nil {
		if err == sql.ErrNoRows {
			return 0, ErrNoDataFound
		}
		return 0, fmt.Errorf("failed to get max revision: %v", err)
	}
	if head.Valid {
		return head.Int64, nil
	}
	return 0, ErrNoDataFound
}

// GoldenCheckpoint gets the latest checkpoint, using the provided function to parse the note data.
func (d *Database) GoldenCheckpoint(parse func([]byte) (*client.Checkpoint, error)) (*client.Checkpoint, error) {
	var datetime sql.NullTime
	var data []byte
	if err := d.db.QueryRow("SELECT datetime, checkpoint FROM checkpoints ORDER BY datetime DESC LIMIT 1").Scan(&datetime, &data); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNoDataFound
		}
		return nil, fmt.Errorf("failed to get latest checkpoint: %w", err)
	}
	if !datetime.Valid {
		return nil, ErrNoDataFound
	}
	return parse(data)
}

// SetGoldenCheckpoint records the given checkpoint to the database.
func (d *Database) SetGoldenCheckpoint(cp *client.Checkpoint) error {
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
		if _, err := tx.Exec("INSERT INTO leaves (id, data) VALUES (?, ?)", lidx, l); err != nil {
			return fmt.Errorf("tx.Exec(): %v", err)
		}
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
	defer func() {
		if err := rows.Close(); err != nil {
			glog.Errorf("rows.Close(): %v", err)
		}
	}()
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("rows.Scan(): %v", err)
		}
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
		if _, err := tx.Exec("INSERT INTO leafMetadata (id, module, version, repohash, modhash) VALUES (?, ?, ?, ?, ?)", midx, m.module, m.version, m.repoHash, m.modHash); err != nil {
			return fmt.Errorf("tx.Exec(): %v", err)
		}
	}
	return tx.Commit()
}

// MaxLeafMetadata gets the ID of the last entry in the leafMetadata table.
// If there is no data, then ErrNoDataFound error is returned.
func (d *Database) MaxLeafMetadata(ctx context.Context) (int64, error) {
	var head sql.NullInt64
	if err := d.db.QueryRow("SELECT MAX(id) AS head FROM leafMetadata").Scan(&head); err != nil {
		if err == sql.ErrNoRows {
			return 0, ErrNoDataFound
		}
		return 0, fmt.Errorf("failed to get max metadata: %v", err)
	}
	if head.Valid {
		return head.Int64, nil
	}
	return 0, ErrNoDataFound
}

// Tile gets the leaf hashes for the given tile, or returns an error.
func (d *Database) Tile(height, level, offset int) ([][]byte, error) {
	var res []byte
	err := d.db.QueryRow("SELECT hashes FROM tiles WHERE height=? AND level=? AND toffset=?", height, level, offset).Scan(&res)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNoDataFound
		}
		return nil, err
	}
	return SplitTile(res, height), nil
}

// SetTile sets the leaf hash data for the given tile.
// The leaf hashes should be 2^height * hashLenBytes long.
func (d *Database) SetTile(height, level, offset int, hashes []byte) error {
	if got, want := len(hashes), (1<<height)*hashLenBytes; got != want {
		return fmt.Errorf("wanted %d tile hash bytes but got %d", want, got)
	}
	_, err := d.db.Exec("INSERT INTO tiles (height, level, toffset, hashes) VALUES (?, ?, ?, ?)", height, level, offset, hashes)
	return err
}

// Duplicates returns summaries of any module@version that appears more than once
// in the log.
func (d *Database) Duplicates() ([]Duplicate, error) {
	var res []Duplicate
	rows, err := d.db.Query("SELECT module, version, COUNT(*) cnt FROM leafMetadata GROUP BY module, version HAVING cnt > 1")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			glog.Errorf("rows.Close(): %v", err)
		}
	}()
	for rows.Next() {
		var module, version string
		var count int
		if err := rows.Scan(&module, &version, &count); err != nil {
			return nil, fmt.Errorf("rows.Scan(): %v", err)
		}
		res = append(res, Duplicate{
			Module:  module,
			Version: version,
			Count:   count,
		})
	}
	return res, nil
}

// SplitTile turns the blob that is the leaf hashes in a tile into separate hashes.
func SplitTile(hashes []byte, height int) [][]byte {
	tileWidth := 1 << height
	res := make([][]byte, tileWidth)
	for i := 0; i < tileWidth; i++ {
		hash := hashes[i*hashLenBytes : (i+1)*hashLenBytes]
		res[i] = hash
	}
	return res
}
