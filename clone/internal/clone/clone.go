// Copyright 2021 Google LLC. All Rights Reserved.
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

// clone contains the core engine for quickly downloading leaves and adding
// them to the database.
package clone

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/clone/internal/download"
	"github.com/google/trillian-examples/clone/logdb"
)

// Cloner is configured with the desired performance parameters, and then performs
// work when Clone is called.
type Cloner struct {
	workers        uint
	fetchBatchSize uint
	writeBatchSize uint

	db *logdb.Database
}

// NewCloner creates a new Cloner with the given performance parameters and database.
func NewCloner(wokers, fetchBatchSize, writeBatchSize uint, db *logdb.Database) Cloner {
	return Cloner{
		workers:        wokers,
		fetchBatchSize: fetchBatchSize,
		writeBatchSize: writeBatchSize,
		db:             db,
	}
}

// Clone downloads all the leaves up to the given treeSize, or returns an error if not possible.
func (c Cloner) Clone(ctx context.Context, treeSize uint64, fetcher download.BatchFetch) error {
	next, err := c.Next()
	if err != nil {
		return fmt.Errorf("couldn't determine first leaf: %v", err)
	}
	if next >= treeSize {
		glog.Infof("No work to do. Local tree size = %d, latest log tree size = %d", next, treeSize)
		return nil
	}

	// Load from the local DB the latest verified checkpoint for the leaves, if any.
	verifiedSize, _, _, err := c.db.GetLatestCheckpoint(ctx)
	if err != nil {
		if err != logdb.ErrNoDataFound {
			return fmt.Errorf("failed to query for any checkpoints: %q", err)
		}
	} else if verifiedSize > next {
		return fmt.Errorf("illegal state: verified size %d > downloaded leaf count %d", verifiedSize, next)
	}

	glog.Infof("Fetching [%d, %d): Remote leaves: %d. Local leaves: %d (%d verified). ", next, treeSize, treeSize, next, verifiedSize)

	// Start downloading leaves in a background thread, with the results put into brc.
	brc := make(chan download.BulkResult, c.writeBatchSize*2)
	go download.Bulk(ctx, next, treeSize, fetcher, c.workers, c.fetchBatchSize, brc)

	// Set up a background thread to report downloading progress (as it can take a long time).
	rCtx, rCtxCancel := context.WithCancel(ctx)
	defer rCtxCancel()
	r := reporter{
		lastReported: next,
		lastReport:   time.Now(),
		treeSize:     treeSize,
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

	// This main thread will now take the downloaded leaves, batch them, and write to the DB.
	batch := make([][]byte, c.writeBatchSize)
	bi := 0
	bs := next
	for br := range brc {
		if br.Err != nil {
			glog.Exit(err)
		}
		workDone := r.trackWork(next)
		next++
		batch[bi] = br.Leaf
		bi = (bi + 1) % int(c.writeBatchSize)
		if bi == 0 {
			if err := c.db.WriteLeaves(ctx, bs, batch); err != nil {
				return fmt.Errorf("failed to write to DB for batch starting at %d: %q", bs, err)
			}
			bs = next
		}
		workDone()
	}
	// All leaves are downloaded, but make sure we write the final (maybe partial) batch.
	if bi > 0 {
		if err := c.db.WriteLeaves(ctx, bs, batch[:bi]); err != nil {
			return fmt.Errorf("failed to write to DB for final batch starting at %d: %q", bs, err)
		}
	}
	// Log a final report and then return.
	r.report()
	glog.Infof("Downloaded %d leaves", bs+uint64(bi))
	return nil
}

func (c Cloner) Next() (uint64, error) {
	// Determine head: the index of the last leaf stored in the DB (or -1 if none present).
	head, err := c.db.Head()
	if err != nil {
		if err == logdb.ErrNoDataFound {
			glog.Infof("Failed to find head of database, assuming empty and starting from scratch: %v", err)
			head = -1
		} else {
			return 0, fmt.Errorf("failed to query for head of local log: %q", err)
		}
	}
	return uint64(head + 1), nil
}

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
