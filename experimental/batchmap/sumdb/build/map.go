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

// map constructs a verifiable map from the modules in Go SumDB.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"reflect"

	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/io/databaseio"
	beamlog "github.com/apache/beam/sdks/v2/go/pkg/beam/log"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/x/beamx"

	"github.com/golang/glog"

	"github.com/google/trillian/experimental/batchmap"

	"github.com/google/trillian-examples/experimental/batchmap/sumdb/build/pipeline"
	"github.com/google/trillian-examples/experimental/batchmap/sumdb/mapdb"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

var (
	sumDBString       = flag.String("sum_db", "", "The connection string to a DB populated by sumdbclone.")
	mapDBString       = flag.String("map_db", "", "Output database where the map tiles will be written.")
	treeID            = flag.Int64("tree_id", 12345, "The ID of the tree. Used as a salt in hashing.")
	prefixStrata      = flag.Int("prefix_strata", 2, "The number of strata of 8-bit strata before the final strata.")
	count             = flag.Int64("count", -1, "The total number of entries starting from the beginning of the SumDB to use, or -1 to use all")
	batchSize         = flag.Int("write_batch_size", 250, "Number of tiles to write per batch")
	incrementalUpdate = flag.Bool("incremental_update", false, "If set the map tiles from the previous revision will be updated with the delta, otherwise this will build the map from scratch each time.")
	buildVersionList  = flag.Bool("build_version_list", false, "If set then the map will also contain a mapping for each module to a log committing to its list of versions.")
)

func init() {
	beam.RegisterType(reflect.TypeOf((*tileToDBRowFn)(nil)).Elem())
	beam.RegisterFunction(tileFromDBRowFn)
	beam.RegisterFunction(pipeline.ParseStatementFn)

	beam.RegisterType(reflect.TypeOf((*logToDBRowFn)(nil)).Elem())
}

func main() {
	flag.Parse()
	beam.Init()

	// Connect to where we will read from and write to.
	sumDB, err := newSumDBMirrorFromFlags()
	if err != nil {
		glog.Exitf("Failed to initialize from local SumDB: %v", err)
	}
	mapDB, rev, err := sinkFromFlags()
	if err != nil {
		glog.Exitf("Failed to initialize Map DB: %v", err)
	}

	pb := pipeline.NewMapBuilder(sumDB, *treeID, *prefixStrata, *buildVersionList)

	beamlog.SetLogger(&BeamGLogger{InfoLogAtVerbosity: 2})
	p, s := beam.NewPipelineWithRoot()

	var tiles, logs beam.PCollection
	var inputLogMetadata pipeline.InputLogMetadata
	if *incrementalUpdate {
		lastMapRev, golden, startID, err := mapDB.LatestRevision()
		if err != nil {
			glog.Exitf("Failed to get LatestRevision: %v", err)
		}
		tileRows := databaseio.Query(s, "sqlite3", *mapDBString, fmt.Sprintf("SELECT * FROM tiles WHERE revision=%d", lastMapRev), reflect.TypeOf(MapTile{}))
		lastTiles := beam.ParDo(s, tileFromDBRowFn, tileRows)

		tiles, inputLogMetadata, err = pb.Update(s, lastTiles, pipeline.InputLogMetadata{
			Checkpoint: golden,
			Entries:    startID,
		}, *count)
		if err != nil {
			glog.Exitf("Failed to build Update pipeline: %v", err)
		}
	} else {
		tiles, logs, inputLogMetadata, err = pb.Create(s, *count)
		if err != nil {
			glog.Exitf("Failed to build Create pipeline: %v", err)
		}
	}

	tileRows := beam.ParDo(s.Scope("convertoutput"), &tileToDBRowFn{Revision: rev}, tiles)
	databaseio.WriteWithBatchSize(s.Scope("sink"), *batchSize, "sqlite3", *mapDBString, "tiles", []string{}, tileRows)

	if *buildVersionList {
		logRows := beam.ParDo(s, &logToDBRowFn{rev}, logs)
		databaseio.WriteWithBatchSize(s.Scope("sinkLogs"), *batchSize, "sqlite3", *mapDBString, "logs", []string{}, logRows)
	}

	// All of the above constructs the pipeline but doesn't run it. Now we run it.
	if err := beamx.Run(context.Background(), p); err != nil {
		glog.Exitf("Failed to execute job: %q", err)
	}

	if err := mapDB.WriteRevision(rev, inputLogMetadata.Checkpoint, inputLogMetadata.Entries); err != nil {
		glog.Exitf("Failed to finalize map revison %d: %v", rev, err)
	}
}

func sinkFromFlags() (*mapdb.TileDB, int, error) {
	if len(*mapDBString) == 0 {
		return nil, 0, fmt.Errorf("missing flag: map_db")
	}

	tiledb, err := mapdb.NewTileDB(*mapDBString)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open map DB at %q: %v", *mapDBString, err)
	}
	if err := tiledb.Init(); err != nil {
		return nil, 0, fmt.Errorf("failed to Init map DB at %q: %v", *mapDBString, err)
	}

	var rev int
	if rev, err = tiledb.NextWriteRevision(); err != nil {
		return nil, 0, fmt.Errorf("failed to query for next write revision: %v", err)

	}
	return tiledb, rev, nil
}

// LogDBRow adapts ModuleVersionLog to the schema format of the Map database to allow for databaseio writing.
type LogDBRow struct {
	Revision int
	Module   string
	Leaves   []byte
}

type logToDBRowFn struct {
	Revision int
}

func (fn *logToDBRowFn) ProcessElement(ctx context.Context, l *pipeline.ModuleVersionLog) (LogDBRow, error) {
	bs, err := json.Marshal(l.Versions)
	if err != nil {
		return LogDBRow{}, err
	}
	return LogDBRow{
		Revision: fn.Revision,
		Module:   l.Module,
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

type sumDBMirror struct {
	dbString string
	db       *sql.DB
}

func newSumDBMirrorFromFlags() (*sumDBMirror, error) {
	if len(*sumDBString) == 0 {
		return nil, fmt.Errorf("missing flag: sum_db")
	}
	db, err := sql.Open("mysql", *sumDBString)
	return &sumDBMirror{
		dbString: *sumDBString,
		db:       db,
	}, err
}

// Head gets the STH and the total number of entries available to process.
func (m *sumDBMirror) Head() ([]byte, int64, error) {
	var cp []byte
	var leafCount int64

	return cp, leafCount, m.db.QueryRow("SELECT size, data FROM checkpoints ORDER BY size DESC LIMIT 1").Scan(&leafCount, &cp)
}

// Entries returns a PCollection of InputLogLeaf, containing entries in range [start, end).
func (m *sumDBMirror) Entries(s beam.Scope, start, end int64) beam.PCollection {
	return databaseio.Query(s, "mysql", m.dbString, fmt.Sprintf("SELECT id, data FROM leaves WHERE id >= %d AND id < %d", start, end), reflect.TypeOf(pipeline.InputLogLeaf{}))
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
