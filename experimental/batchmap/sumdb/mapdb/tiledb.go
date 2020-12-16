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

package mapdb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/trillian/experimental/batchmap"
)

// NoRevisionsFound is returned when the DB appears valid but has no revisions in it.
type NoRevisionsFound = error

// TileDB provides read/write access to the generated Map tiles.
type TileDB struct {
	db *sql.DB
}

// NewTileDB creates a TileDB using a file at the given location.
// If the file doesn't exist it will be created.
func NewTileDB(location string) (*TileDB, error) {
	db, err := sql.Open("sqlite3", location)
	if err != nil {
		return nil, err
	}
	return &TileDB{
		db: db,
	}, nil
}

// Init creates the database tables if needed.
func (d *TileDB) Init() error {
	// TODO(mhutchinson): Consider storing the entries too:
	// CREATE TABLE IF NOT EXISTS entries (revision INTEGER, keyhash BLOB, key STRING, value STRING, PRIMARY KEY (revision, keyhash))

	if _, err := d.db.Exec("CREATE TABLE IF NOT EXISTS revisions (revision INTEGER PRIMARY KEY, datetime TIMESTAMP, logroot BLOB, count INTEGER)"); err != nil {
		return err
	}
	if _, err := d.db.Exec("CREATE TABLE IF NOT EXISTS tiles (revision INTEGER, path BLOB, tile BLOB, PRIMARY KEY (revision, path))"); err != nil {
		return err
	}
	return nil
}

// NextWriteRevision gets the revision that the next generation of the map should be written at.
func (d *TileDB) NextWriteRevision() (int, error) {
	var rev sql.NullInt32
	if err := d.db.QueryRow("SELECT MAX(revision) FROM tiles").Scan(&rev); err != nil {
		return 0, fmt.Errorf("failed to get max revision: %v", err)
	}
	if rev.Valid {
		return int(rev.Int32) + 1, nil
	}
	return 0, nil
}

// LatestRevision gets the metadata for the last completed write.
func (d *TileDB) LatestRevision() (rev int, logroot []byte, count int64, err error) {
	var sqlRev sql.NullInt32
	if err := d.db.QueryRow("SELECT revision, logroot, count FROM revisions ORDER BY revision DESC LIMIT 1").Scan(&sqlRev, &logroot, &count); err != nil {
		return 0, nil, 0, fmt.Errorf("failed to get latest revision: %v", err)
	}
	if sqlRev.Valid {
		return int(sqlRev.Int32), logroot, count, nil
	}
	return 0, nil, 0, NoRevisionsFound(errors.New("no revisions found"))
}

// Tile gets the tile at the given path in the given revision of the map.
func (d *TileDB) Tile(revision int, path []byte) (*batchmap.Tile, error) {
	var bs []byte
	if err := d.db.QueryRow("SELECT tile FROM tiles WHERE revision=? AND path=?", revision, path).Scan(&bs); err != nil {
		return nil, err
	}
	tile := &batchmap.Tile{}
	if err := json.Unmarshal(bs, tile); err != nil {
		return nil, fmt.Errorf("failed to parse tile at revision=%d, path=%x: %v", revision, path, err)
	}
	return tile, nil
}

// WriteRevision writes the metadata for a completed run into the database.
// If this method isn't called then the tiles may be written but this revision will be
// skipped by sensible readers because the provenance information isn't available.
func (d *TileDB) WriteRevision(rev int, logCheckpoint []byte, count int64) error {
	now := time.Now()
	_, err := d.db.Exec("INSERT INTO revisions (revision, datetime, logroot, count) VALUES (?, ?, ?, ?)", rev, now, logCheckpoint, count)
	if err != nil {
		return fmt.Errorf("failed to write revision: %w", err)
	}
	return nil
}
