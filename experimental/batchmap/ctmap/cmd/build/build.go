// Copyright 2021 Google LLC. All Rights Reserved.
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

// build is a tool to build a map from a given clone of a log.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"reflect"
	"strings"

	"github.com/apache/beam/sdks/go/pkg/beam"
	"github.com/apache/beam/sdks/go/pkg/beam/io/databaseio"
	"github.com/apache/beam/sdks/go/pkg/beam/io/filesystem"
	"github.com/apache/beam/sdks/go/pkg/beam/io/textio"
	beamlog "github.com/apache/beam/sdks/go/pkg/beam/log"
	"github.com/apache/beam/sdks/go/pkg/beam/x/beamx"
	"github.com/golang/glog"
	"github.com/google/trillian-examples/clone/logdb"
	"github.com/google/trillian-examples/experimental/batchmap/ctmap/internal/pipeline"
	"github.com/google/trillian/experimental/batchmap"

	_ "github.com/go-sql-driver/mysql"
)

var (
	mysqlURI         = flag.String("mysql_log_uri", "", "URL of a MySQL database to read the log from.")
	mapOutputRootDir = flag.String("map_output_root_dir", "", "Filesystem root for this map. Cannot be shared with other maps.")
	treeID           = flag.Int64("tree_id", 12345, "The ID of the tree. Used as a salt in hashing.")
	prefixStrata     = flag.Int("prefix_strata", 2, "The number of strata of 8-bit strata before the final strata.")
	count            = flag.Int64("count", 1, "The total number of entries starting from the beginning of the log to use")
)

func init() {
	beam.RegisterType(reflect.TypeOf((*writeTileFn)(nil)).Elem())
	beam.RegisterType(reflect.TypeOf((*writeCheckpointFn)(nil)).Elem())
}

func main() {
	flag.Parse()
	beam.Init()

	ctx := context.Background()
	if !strings.HasSuffix(*mapOutputRootDir, "/") {
		glog.Exitf("Required flag 'map_output_root_dir' must end with '/' (got %q)", *mapOutputRootDir)
	}
	if len(*mysqlURI) == 0 {
		glog.Exit("Missing required flag 'mysql_log_uri'")
	}
	db, err := logdb.NewDatabase(*mysqlURI)
	if err != nil {
		glog.Exitf("Failed to connect to database: %q", err)
	}

	beamlog.SetLogger(&BeamGLogger{InfoLogAtVerbosity: 2})
	p, s := beam.NewPipelineWithRoot()

	log := mySQLLog{
		db:       db,
		dbString: *mysqlURI,
	}
	mb := pipeline.NewMapBuilder(&log, *treeID, *prefixStrata)
	r, err := mb.Create(ctx, s, *count)
	if err != nil {
		glog.Exitf("Failed to create pipeline: %v", err)
	}
	// Write out the leaf values, i.e. the logs.
	// This currently writes a single large file containing all the results.
	textio.Write(s, *mapOutputRootDir+"logs.txt", beam.ParDo(s, formatFn, r.DomainCertIndexLogs))

	// Write out all of the tiles that represent the map.
	beam.ParDo0(s, &writeTileFn{*mapOutputRootDir}, r.MapTiles)

	//Write out the map checkpoint.
	beam.ParDo0(s, &writeCheckpointFn{
		RootDir:       *mapOutputRootDir,
		LogCheckpoint: r.Metadata.Checkpoint,
		EntryCount:    uint64(r.Metadata.Entries),
	}, r.MapTiles)

	glog.Info("Pipeline constructed, calling beamx.Run()")
	// All of the above constructs the pipeline but doesn't run it. Now we run it.
	if err := beamx.Run(context.Background(), p); err != nil {
		glog.Exitf("Failed to execute job: %q", err)
	}
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

type mySQLLog struct {
	db       *logdb.Database
	dbString string
}

// Head returns the metadata of available entries.
func (l *mySQLLog) Head(ctx context.Context) ([]byte, int64, error) {
	// There may be more leaves than `size`, but any leaves at a higher index than
	// this have not been verified, and are not committed to by the checkpoint so
	// we can't use them in the map.
	size, cp, _, err := l.db.GetLatestCheckpoint(ctx)
	return cp, int64(size), err
}

// Entries returns a PCollection of InputLogLeaf, containing entries in range [start, end).
func (l *mySQLLog) Entries(s beam.Scope, start, end int64) beam.PCollection {
	return databaseio.Query(s, "mysql", l.dbString, fmt.Sprintf("SELECT id AS Seq, data AS Data FROM leaves WHERE id >= %d AND id < %d", start, end), reflect.TypeOf(pipeline.InputLogLeaf{}))
}

// formatFn is a DoFn that formats a domain's log as a string.
func formatFn(l *pipeline.DomainCertIndexLog) string {
	r := l.Domain
	for _, i := range l.Indices {
		r = fmt.Sprintf("%s,%d", r, i)
	}
	return r
}

type writeTileFn struct {
	RootDir string
}

func (w *writeTileFn) ProcessElement(ctx context.Context, t *batchmap.Tile) error {
	filename := fmt.Sprintf("%stile-%x", w.RootDir, t.Path)
	fs, err := filesystem.New(ctx, filename)
	if err != nil {
		return err
	}
	defer fs.Close()

	fd, err := fs.OpenWrite(ctx, filename)
	if err != nil {
		return err
	}
	buf := bufio.NewWriterSize(fd, 1<<20) // use 1MB buffer

	beamlog.Infof(ctx, "Writing to %v", filename)

	for _, l := range t.Leaves {
		if _, err := buf.Write(l.Path); err != nil {
			return err
		}
		if _, err := buf.Write([]byte{'\t'}); err != nil {
			return err
		}
		if _, err := buf.Write(l.Hash); err != nil {
			return err
		}
		if _, err := buf.Write([]byte{'\n'}); err != nil {
			return err
		}
	}

	if err := buf.Flush(); err != nil {
		return err
	}
	return fd.Close()
}

type writeCheckpointFn struct {
	RootDir       string
	LogCheckpoint []byte
	EntryCount    uint64
}

func (w *writeCheckpointFn) ProcessElement(ctx context.Context, t *batchmap.Tile) error {
	if len(t.Path) > 0 {
		return nil
	}
	root := t.RootHash

	filename := fmt.Sprintf("%scheckpoint", w.RootDir)
	fs, err := filesystem.New(ctx, filename)
	if err != nil {
		return err
	}
	defer fs.Close()

	fd, err := fs.OpenWrite(ctx, filename)
	if err != nil {
		return err
	}

	fd.Write([]byte(fmt.Sprintf("%d\n%x\n", w.EntryCount, root)))
	fd.Write(w.LogCheckpoint)

	// TODO(mhutchinson): Add signature to the map root.

	return fd.Close()
}
