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
	"errors"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/google/trillian/experimental/batchmap/tilepb"
)

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

	// TODO(mhutchinson): Store revisions table, something like:
	// CREATE TABLE IF NOT EXISTS revisions (revision INTEGER PRIMARY KEY, logroot BLOB, count INTEGER)
	if _, err := d.db.Exec("CREATE TABLE IF NOT EXISTS tiles (revision INTEGER, path BLOB, tile BLOB, PRIMARY KEY (revision, path))"); err != nil {
		return err
	}
	return nil
}

// MaxRevision gets the largest revision currently in the DB. Fails with NoRevisionsFound
// if the DB was queried successfully but was empty, and a generic error on other failures.
func (d *TileDB) MaxRevision() (int, error) {
	var rev sql.NullInt32
	if err := d.db.QueryRow("SELECT MAX(revision) FROM tiles").Scan(&rev); err != nil {
		return 0, fmt.Errorf("failed to get max revision: %v", err)
	}
	if rev.Valid {
		return int(rev.Int32), nil
	}
	return 0, NoRevisionsFound(errors.New("no revisions found"))
}

// Tile gets the tile at the given path in the given revision of the map.
func (d *TileDB) Tile(revision int, path []byte) (*tilepb.Tile, error) {
	var bs []byte
	var err error
	if len(path) == 0 {
		err = d.db.QueryRow("SELECT tile FROM tiles WHERE revision=? AND path IS NULL", revision).Scan(&bs)
	} else {
		err = d.db.QueryRow("SELECT tile FROM tiles WHERE revision=? AND path=?", revision, path).Scan(&bs)
	}
	if err != nil {
		return nil, err
	}
	tile := &tilepb.Tile{}
	if err := proto.Unmarshal(bs, tile); err != nil {
		return nil, fmt.Errorf("failed to parse tile at revision=%d, path=%x: %v", revision, path, err)
	}
	return tile, nil
}
