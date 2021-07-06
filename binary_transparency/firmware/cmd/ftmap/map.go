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
	"github.com/google/trillian/types"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/ftmap"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

var (
	trillianMySQL = flag.String("trillian_mysql", "", "The connection string to the Trillian MySQL database.")
	mapDBString   = flag.String("map_db", "", "Connection path for output database where the map tiles will be written.")
	count         = flag.Int64("count", -1, "The total number of entries starting from the beginning of the log to use, or -1 to use all. This can be used to independently create maps of the same size.")
	batchSize     = flag.Int("write_batch_size", 250, "Number of tiles to write per batch")
)

func init() {
	beam.RegisterType(reflect.TypeOf((*tileToDBRowFn)(nil)).Elem())
	beam.RegisterType(reflect.TypeOf((*logToDBRowFn)(nil)).Elem())
	beam.RegisterType(reflect.TypeOf((*aggToDBRowFn)(nil)).Elem())
}

func main() {
	flag.Parse()
	beam.Init()

	// Connect to where we will read from and write to.
	trillianDB, err := newTrillianDBFromFlags()
	if err != nil {
		glog.Exitf("Failed to initialize Trillian connection: %v", err)
	}
	mapDB, rev, err := sinkFromFlags()
	if err != nil {
		glog.Exitf("Failed to initialize Map DB: %v", err)
	}

	// The tree & strata config is part of the API for clients. If we make this configurable then
	// there needs to be some dynamic way to get this to clients (e.g. in a MapCheckpoint).
	pb := ftmap.NewMapBuilder(trillianDB, api.MapTreeID, api.MapPrefixStrata)

	beamlog.SetLogger(&BeamGLogger{InfoLogAtVerbosity: 2})
	p, s := beam.NewPipelineWithRoot()
	result, err := pb.Create(s, *count)
	if err != nil {
		glog.Exitf("Failed to build Create pipeline: %v", err)
	}

	tileRows := beam.ParDo(s.Scope("convertTiles"), &tileToDBRowFn{Revision: rev}, result.MapTiles)
	databaseio.WriteWithBatchSize(s.Scope("sinkTiles"), *batchSize, "sqlite3", *mapDBString, "tiles", []string{}, tileRows)
	aggRows := beam.ParDo(s.Scope("convertAgg"), &aggToDBRowFn{Revision: rev}, result.AggregatedFirmware)
	databaseio.WriteWithBatchSize(s.Scope("sinkAgg"), *batchSize, "sqlite3", *mapDBString, "aggregations", []string{}, aggRows)
	logRows := beam.ParDo(s, &logToDBRowFn{rev}, result.DeviceLogs)
	databaseio.WriteWithBatchSize(s.Scope("sinkLogs"), *batchSize, "sqlite3", *mapDBString, "logs", []string{}, logRows)

	// All of the above constructs the pipeline but doesn't run it. Now we run it.
	if err := beamx.Run(context.Background(), p); err != nil {
		glog.Exitf("Failed to execute job: %q", err)
	}

	// Now write the revision metadata to finalize this map construction.
	if err := mapDB.WriteRevision(rev, result.Metadata.Checkpoint, result.Metadata.Entries); err != nil {
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

func (fn *logToDBRowFn) ProcessElement(ctx context.Context, l *api.DeviceReleaseLog) (LogDBRow, error) {
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

// AggregatedFirmwareDBRow adapts AggregatedFirmware to the schema format of the Map database to allow for databaseio writing.
type AggregatedFirmwareDBRow struct {
	// The keys are the index of the FW Log Metadata that was aggregated, and map Revision number.
	FWLogIndex uint64
	Revision   int

	// The value is the summary of the aggregated information. Thus far, a bool for whether it's considered good.
	// Clients will have the other information about the FW so no need to duplicate it here.
	Good int
}

type aggToDBRowFn struct {
	Revision int
}

func (fn *aggToDBRowFn) ProcessElement(ctx context.Context, t *api.AggregatedFirmware) AggregatedFirmwareDBRow {
	goodInt := 0
	if t.Good {
		goodInt = 1
	}
	return AggregatedFirmwareDBRow{
		FWLogIndex: t.Index,
		Revision:   fn.Revision,
		Good:       goodInt,
	}
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
	// This implementation taken from Trillian's storage/mysql/log_storage.go#fetchLatestRoot
	var timestamp, treeSize, treeRevision int64
	var rootHash []byte
	if err := m.db.QueryRow("SELECT TreeHeadTimestamp,TreeSize,RootHash,TreeRevision FROM TreeHead ORDER BY TreeRevision DESC LIMIT 1").Scan(
		&timestamp, &treeSize, &rootHash, &treeRevision,
	); err != nil {
		// It's possible there are no roots for this tree yet
		return []byte{}, 0, fmt.Errorf("failed to read TreeHead table: %w", err)
	}

	// Put logRoot back together. Fortunately LogRoot has a deterministic serialization.
	cp, err := (&types.LogRootV1{
		RootHash:       rootHash,
		TimestampNanos: uint64(timestamp),
		Revision:       uint64(treeRevision),
		TreeSize:       uint64(treeSize),
	}).MarshalBinary()
	if err != nil {
		return []byte{}, 0, fmt.Errorf("failed to marshal LogRoot: %w", err)
	}
	return cp, treeSize, nil
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
