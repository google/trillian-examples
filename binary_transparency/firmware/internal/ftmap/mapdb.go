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

package ftmap

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian/experimental/batchmap"
	"github.com/google/trillian/types"
)

// NoRevisionsFound is returned when the DB appears valid but has no revisions in it.
type NoRevisionsFound = error

// MapDB provides read/write access to the generated Map tiles.
type MapDB struct {
	db *sql.DB
}

// NewMapDB creates a MapDB using a file at the given location.
// If the file doesn't exist it will be created.
func NewMapDB(location string) (*MapDB, error) {
	db, err := sql.Open("sqlite3", location)
	if err != nil {
		return nil, err
	}
	mapDB := &MapDB{
		db: db,
	}
	return mapDB, mapDB.Init()
}

// Init creates the database tables if needed.
func (d *MapDB) Init() error {
	if _, err := d.db.Exec("CREATE TABLE IF NOT EXISTS revisions (revision INTEGER PRIMARY KEY, datetime TIMESTAMP, logroot BLOB, count INTEGER)"); err != nil {
		return err
	}
	if _, err := d.db.Exec("CREATE TABLE IF NOT EXISTS tiles (revision INTEGER, path BLOB, tile BLOB, PRIMARY KEY (revision, path))"); err != nil {
		return err
	}
	if _, err := d.db.Exec("CREATE TABLE IF NOT EXISTS logs (deviceID BLOB, revision INTEGER, leaves BLOB, PRIMARY KEY (deviceID, revision))"); err != nil {
		return err
	}
	// We use an INTEGER for a boolean to make life easy across multiple DB implementations.
	if _, err := d.db.Exec("CREATE TABLE IF NOT EXISTS aggregations (fwLogIndex INTEGER, revision INTEGER, good INTEGER, PRIMARY KEY (fwLogIndex, revision))"); err != nil {
		return err
	}
	return nil
}

// NextWriteRevision gets the revision that the next generation of the map should be written at.
func (d *MapDB) NextWriteRevision() (int, error) {
	var rev sql.NullInt32
	// TODO(mhutchinson): This should be updated to include the max of "tiles" or "logs".
	if err := d.db.QueryRow("SELECT MAX(revision) FROM tiles").Scan(&rev); err != nil {
		return 0, fmt.Errorf("failed to get max revision: %v", err)
	}
	if rev.Valid {
		return int(rev.Int32) + 1, nil
	}
	return 0, nil
}

// LatestRevision gets the metadata for the last completed write.
func (d *MapDB) LatestRevision() (rev int, logroot types.LogRootV1, count int64, err error) {
	var sqlRev sql.NullInt32
	var lcpRaw []byte
	if err := d.db.QueryRow("SELECT revision, logroot, count FROM revisions ORDER BY revision DESC LIMIT 1").Scan(&sqlRev, &lcpRaw, &count); err != nil {
		return 0, types.LogRootV1{}, 0, fmt.Errorf("failed to get latest revision: %v", err)
	}
	if sqlRev.Valid {
		if err := logroot.UnmarshalBinary(lcpRaw); err != nil {
			return 0, types.LogRootV1{}, 0, fmt.Errorf("failed to get unmarshal log root: %v", err)
		}
		return int(sqlRev.Int32), logroot, count, nil
	}
	return 0, types.LogRootV1{}, 0, NoRevisionsFound(errors.New("no revisions found"))
}

// Tile gets the tile at the given path in the given revision of the map.
func (d *MapDB) Tile(revision int, path []byte) (*batchmap.Tile, error) {
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

// Aggregation gets the aggregation for the firmware at the given log index.
func (d *MapDB) Aggregation(revision int, fwLogIndex uint64) (api.AggregatedFirmware, error) {
	var good int
	if err := d.db.QueryRow("SELECT good FROM aggregations WHERE fwLogIndex=? AND revision=?", fwLogIndex, revision).Scan(&good); err != nil {
		return api.AggregatedFirmware{}, err
	}
	return api.AggregatedFirmware{
		Index: fwLogIndex,
		Good:  good > 0,
	}, nil
}

// WriteRevision writes the metadata for a completed run into the database.
// If this method isn't called then the tiles may be written but this revision will be
// skipped by sensible readers because the provenance information isn't available.
func (d *MapDB) WriteRevision(rev int, logCheckpoint []byte, count int64) error {
	now := time.Now()
	_, err := d.db.Exec("INSERT INTO revisions (revision, datetime, logroot, count) VALUES (?, ?, ?, ?)", rev, now, logCheckpoint, count)
	if err != nil {
		return fmt.Errorf("failed to write revision: %w", err)
	}
	return nil
}

// Versions gets the log of versions for the given module in the given map revision.
func (d *MapDB) Versions(revision int, module string) ([]string, error) {
	var bs []byte
	if err := d.db.QueryRow("SELECT leaves FROM logs WHERE revision=? AND module=?", revision, module).Scan(&bs); err != nil {
		return nil, err
	}
	var versions []string
	if err := json.Unmarshal(bs, &versions); err != nil {
		return nil, fmt.Errorf("failed to parse tile at revision=%d, path=%x: %v", revision, module, err)
	}
	return versions, nil
}
