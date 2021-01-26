// Copyright 2020 Google LLC. All Rights Reserved.
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

// map constructs a verifiable map from the firmware in the FT log.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"reflect"

	"github.com/apache/beam/sdks/go/pkg/beam"
	"github.com/apache/beam/sdks/go/pkg/beam/io/databaseio"
	beamlog "github.com/apache/beam/sdks/go/pkg/beam/log"
	"github.com/apache/beam/sdks/go/pkg/beam/x/beamx"

	"github.com/golang/glog"

	"github.com/google/trillian/experimental/batchmap"

	"github.com/google/trillian-examples/binary_transparency/firmware/internal/ftmap"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

var (
	trillianMySQL = flag.String("trillian_mysql", "", "The connection string to the Trillian MySQL database.")
	mapDBString   = flag.String("map_db", "", "File path for output database where the map tiles will be written.") // TODO(mhutchinson): this needs to be able to sink to a client/server database
	count         = flag.Int64("count", -1, "The total number of entries starting from the beginning of the log to use, or -1 to use all. This can be used to independently create maps of the same size.")
	treeID        = flag.Int64("tree_id", 12345, "The ID of the tree. Used as a salt in hashing.")
	prefixStrata  = flag.Int("prefix_strata", 2, "The number of strata of 8-bit strata before the final strata.")
	batchSize     = flag.Int("write_batch_size", 250, "Number of tiles to write per batch")
)

func init() {
	beam.RegisterType(reflect.TypeOf((*tileToDBRowFn)(nil)).Elem())
	beam.RegisterType(reflect.TypeOf((*logToDBRowFn)(nil)).Elem())
}

func main() {
	flag.Parse()

	// Connect to where we will read from and write to.
	sumDB, err := newTrillianDBFromFlags()
	if err != nil {
		glog.Exitf("Failed to initialize from local SumDB: %v", err)
	}
	mapDB, rev, err := sinkFromFlags()
	if err != nil {
		glog.Exitf("Failed to initialize Map DB: %v", err)
	}

	pb := ftmap.NewMapBuilder(sumDB, *treeID, *prefixStrata)

	beam.Init()
	beamlog.SetLogger(&BeamGLogger{InfoLogAtVerbosity: 2})
	p, s := beam.NewPipelineWithRoot()

	var tiles, logs beam.PCollection
	var inputLogMetadata ftmap.InputLogMetadata

	tiles, logs, inputLogMetadata, err = pb.Create(s, *count)
	if err != nil {
		glog.Exitf("Failed to build Create pipeline: %v", err)
	}

	tileRows := beam.ParDo(s.Scope("convertoutput"), &tileToDBRowFn{Revision: rev}, tiles)
	databaseio.WriteWithBatchSize(s.Scope("sink"), *batchSize, "sqlite3", *mapDBString, "tiles", []string{}, tileRows)
	logRows := beam.ParDo(s, &logToDBRowFn{rev}, logs)
	databaseio.WriteWithBatchSize(s.Scope("sinkLogs"), *batchSize, "sqlite3", *mapDBString, "logs", []string{}, logRows)

	// All of the above constructs the pipeline but doesn't run it. Now we run it.
	if err := beamx.Run(context.Background(), p); err != nil {
		glog.Exitf("Failed to execute job: %q", err)
	}

	// Now write the revision metadata to finalize this map construction.
	if err := mapDB.WriteRevision(rev, inputLogMetadata.Checkpoint, inputLogMetadata.Entries); err != nil {
		glog.Exitf("Failed to finalize map revison %d: %v", rev, err)
	}
}

func sinkFromFlags() (*ftmap.MapDB, int, error) {
	if len(*mapDBString) == 0 {
		return nil, 0, fmt.Errorf("missing flag: map_db")
	}

	mapDB, err := ftmap.NewMapDB(*mapDBString)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open map DB at %q: %v", *mapDBString, err)
	}
	if err := mapDB.Init(); err != nil {
		return nil, 0, fmt.Errorf("failed to Init map DB at %q: %v", *mapDBString, err)
	}

	var rev int
	if rev, err = mapDB.NextWriteRevision(); err != nil {
		return nil, 0, fmt.Errorf("failed to query for next write revision: %v", err)

	}
	return mapDB, rev, nil
}

// LogDBRow adapts DeviceReleaseLog to the schema format of the Map database to allow for databaseio writing.
type LogDBRow struct {
	Revision int
	DeviceID string
	Leaves   []byte
}

type logToDBRowFn struct {
	Revision int
}

func (fn *logToDBRowFn) ProcessElement(ctx context.Context, l *ftmap.DeviceReleaseLog) (LogDBRow, error) {
	bs, err := json.Marshal(l.Revisions)
	if err != nil {
		return LogDBRow{}, err
	}
	return LogDBRow{
		Revision: fn.Revision,
		DeviceID: l.DeviceID,
		Leaves:   bs,
	}, nil
}

// MapTile is the schema format of the Map database to allow for databaseio writing.
type MapTile struct {
	Revision int
	Path     []byte
	Tile     []byte
}

type tileToDBRowFn struct {
	Revision int
}

func (fn *tileToDBRowFn) ProcessElement(ctx context.Context, t *batchmap.Tile) (MapTile, error) {
	bs, err := json.Marshal(t)
	if err != nil {
		return MapTile{}, err
	}
	return MapTile{
		Revision: fn.Revision,
		Path:     t.Path,
		Tile:     bs,
	}, nil
}

func tileFromDBRowFn(t MapTile) (*batchmap.Tile, error) {
	var res batchmap.Tile
	if err := json.Unmarshal(t.Tile, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// TODO(mhutchinson): This only works if the Trillian DB has a single tree.
type trillianDB struct {
	dbString string
	db       *sql.DB
}

func newTrillianDBFromFlags() (*trillianDB, error) {
	if len(*trillianMySQL) == 0 {
		return nil, fmt.Errorf("missing flag: trillian_mysql")
	}
	db, err := sql.Open("mysql", *trillianMySQL)
	return &trillianDB{
		dbString: *trillianMySQL,
		db:       db,
	}, err
}

// Head gets the STH and the total number of entries available to process.
func (m *trillianDB) Head() ([]byte, int64, error) {
	var cp []byte
	var leafCount int64

	// TODO(mhutchinson): This should construct the full signed root; see Trillian's storage/mysql/log_storage.go#fetchLatestRoot
	return cp, leafCount, m.db.QueryRow("SELECT TreeSize, RootHash From TreeHead ORDER BY TreeRevision DESC LIMIT 1").Scan(&leafCount, &cp)
}

const sequencedLeafDataQuery = `
SELECT
  s.SequenceNumber AS Seq,
  l.LeafValue AS Data
FROM SequencedLeafData s INNER JOIN LeafData l
  ON s.TreeId = l.TreeId AND s.LeafIdentityHash = l.LeafIdentityHash 
WHERE
  s.SequenceNumber >= %d AND s.SequenceNumber < %d
`

// Entries returns a PCollection of InputLogLeaf, containing entries in range [start, end).
func (m *trillianDB) Entries(s beam.Scope, start, end int64) beam.PCollection {
	return databaseio.Query(s, "mysql", m.dbString, fmt.Sprintf(sequencedLeafDataQuery, start, end), reflect.TypeOf(ftmap.InputLogLeaf{}))
}

// BeamGLogger allows Beam to log via the glog mechanism.
// This is used to allow the very verbose logging output from Beam to be switched off.
type BeamGLogger struct {
	InfoLogAtVerbosity glog.Level
}

// Log logs.
func (l *BeamGLogger) Log(ctx context.Context, sev beamlog.Severity, _ int, msg string) {
	switch sev {
	case beamlog.SevDebug:
		glog.V(3).Info(msg)
	case beamlog.SevInfo:
		glog.V(l.InfoLogAtVerbosity).Info(msg)
	case beamlog.SevError:
		glog.Error(msg)
	case beamlog.SevWarn:
		glog.Warning(msg)
	default:
		glog.V(5).Infof("?? %s", msg)
	}
}
