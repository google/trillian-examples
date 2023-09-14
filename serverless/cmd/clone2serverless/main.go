// Copyright 2023 Google LLC. All Rights Reserved.
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

// clone2serverless is a one-shot tool that creates a tile-based (serverless)
// log on disk from the contents of a cloned DB.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/clone/logdb"
	"github.com/google/trillian-examples/serverless/cmd/clone2serverless/internal/storage/fs"
	fmtlog "github.com/transparency-dev/formats/log"
	"github.com/transparency-dev/merkle/rfc6962"
	"github.com/transparency-dev/serverless/pkg/log"
	"golang.org/x/mod/sumdb/note"

	_ "github.com/go-sql-driver/mysql"
)

var (
	mysqlURI   = flag.String("mysql_uri", "", "URL of a MySQL database to clone the log into. The DB should contain only one log.")
	outputRoot = flag.String("output_root", "", "File path to the directory that the tile-based log will be created in. If this directory exists then it must contain a valid tile-based log. If it doesn't exist it will be created.")
	vkey       = flag.String("log_vkey", "", "Verifier key for the log checkpoint signature.")
	origin     = flag.String("log_origin", "", "Expected Origin for the log checkpoints.")
)

func main() {
	flag.Parse()

	if len(*mysqlURI) == 0 {
		glog.Exit("Missing required parameter 'mysql_uri'")
	}
	if len(*origin) == 0 {
		glog.Exit("Missing required parameter 'log_origin'")
	}
	if len(*vkey) == 0 {
		glog.Exit("Missing required parameter 'log_vkey'")
	}
	if len(*outputRoot) == 0 {
		glog.Exit("Missing required parameter 'output_root'")
	}

	db, err := logdb.NewDatabase(*mysqlURI)
	if err != nil {
		glog.Exitf("Failed to connect to database: %v", err)
	}
	logVerifier, err := note.NewVerifier(*vkey)
	if err != nil {
		glog.Exitf("Failed to instantiate Verifier: %q", err)
	}

	ctx := context.Background()
	toSize, inputCheckpointRaw, _, err := db.GetLatestCheckpoint(ctx)
	if err != nil {
		glog.Exitf("Failed to get latest checkpoint from clone DB: %v", err)
	}
	logRoot, fromSize, err := initStorage(logVerifier, *outputRoot)
	if err != nil {
		glog.Exitf("Could not initialize tile-based storage: %v", err)
	}

	// Set up a background thread to report progress (as it can take a long time).
	rCtx, rCtxCancel := context.WithCancel(ctx)
	defer rCtxCancel()
	r := reporter{
		lastReported: fromSize,
		lastReport:   time.Now(),
		treeSize:     toSize,
	}
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				r.report()
			case <-rCtx.Done():
				return
			}
		}
	}()

	results := make(chan logdb.StreamResult, 16)
	glog.V(1).Infof("Sequencing leaves [%d, %d)", fromSize, toSize)
	go db.StreamLeaves(ctx, fromSize, toSize, results)

	index := fromSize
	for result := range results {
		if result.Err != nil {
			glog.Exitf("Failed fetching leaves from DB: %v", result.Err)
		}
		workDone := r.trackWork(index)
		err := logRoot.Assign(ctx, index, result.Leaf)
		if err != nil {
			glog.Exitf("Failed to sequence leaves: %v", err)
		}
		workDone()
		index++
	}
	rCtxCancel()

	glog.V(1).Infof("Integrating leaves [%d, %d)", fromSize, toSize)
	if _, err := log.Integrate(ctx, fromSize, logRoot, rfc6962.DefaultHasher); err != nil {
		glog.Exitf("Failed to integrate leaves: %v", err)
	}
	glog.V(1).Info("Writing checkpoint")
	if err := logRoot.WriteCheckpoint(ctx, inputCheckpointRaw); err != nil {
		glog.Exitf("Failed to write checkpoint: %v", err)
	}
	glog.V(1).Infof("Successfully created tlog with %d leaves at %q", toSize, *outputRoot)
}

// initStorage loads or creates the tile-based log storage in the given output directory
// and returns the storage, along with the tree size currently persisted.
func initStorage(logVerifier note.Verifier, outputRoot string) (*fs.Storage, uint64, error) {
	var logRoot *fs.Storage
	var fromSize uint64
	if fstat, err := os.Stat(outputRoot); os.IsNotExist(err) {
		glog.Infof("No directory exists at %q; creating new tile-based log filesystem", outputRoot)
		logRoot, err = fs.Create(outputRoot)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create log root filesystem: %v", err)
		}
		glog.Infof("Created new log root filesystem at %q", outputRoot)
	} else if err != nil {
		return nil, 0, fmt.Errorf("failed to stat %q: %v", outputRoot, err)
	} else if !fstat.IsDir() {
		return nil, 0, fmt.Errorf("output root is not a directory: %v", outputRoot)
	} else {
		glog.Infof("Reading existing tile-based log from %q", outputRoot)
		fsCheckpointRaw, err := fs.ReadCheckpoint(outputRoot)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to read checkpoint from log root filesystem: %v", err)
		}
		fsCheckpoint, _, _, err := fmtlog.ParseCheckpoint(fsCheckpointRaw, *origin, logVerifier)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to open Checkpoint: %q", err)
		}
		logRoot, err = fs.Load(outputRoot, fromSize)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to load log root filesystem: %v", err)
		}
		fromSize = fsCheckpoint.Size
	}
	return logRoot, fromSize, nil
}

// reporter is copied from clone/internal/cloner/clone.go.
type reporter struct {
	// treeSize is fixed for the lifetime of the reporter.
	treeSize uint64

	// Fields read/written only in report()
	lastReport   time.Time
	lastReported uint64

	// Fields shared across multiple threads, protected by workedMutex
	lastWorked  uint64
	epochWorked time.Duration
	workedMutex sync.Mutex
}

func (r *reporter) report() {
	lastWorked, epochWorked := func() (uint64, time.Duration) {
		r.workedMutex.Lock()
		defer r.workedMutex.Unlock()
		lw, ew := r.lastWorked, r.epochWorked
		r.epochWorked = 0
		return lw, ew
	}()

	elapsed := time.Since(r.lastReport)
	workRatio := epochWorked.Seconds() / elapsed.Seconds()
	remaining := r.treeSize - r.lastReported - 1
	rate := float64(lastWorked-r.lastReported) / elapsed.Seconds()
	eta := time.Duration(float64(remaining)/rate) * time.Second
	glog.Infof("%.1f leaves/s, last leaf=%d (remaining: %d, ETA: %s), time working=%.1f%%", rate, r.lastReported, remaining, eta, 100*workRatio)

	r.lastReport = time.Now()
	r.lastReported = r.lastWorked
}

func (r *reporter) trackWork(index uint64) func() {
	start := time.Now()

	return func() {
		end := time.Now()
		r.workedMutex.Lock()
		defer r.workedMutex.Unlock()
		r.lastWorked = index
		r.epochWorked += end.Sub(start)
	}
}
