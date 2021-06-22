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

// ctclone is a one-shot tool for downloading entries from a CT log.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/clone/internal/download"
	"github.com/google/trillian-examples/clone/logdb"

	_ "github.com/go-sql-driver/mysql"
)

const getEntriesFormat = "ct/v1/get-entries?start=%d&end=%d"

var (
	logURL         = flag.String("log_url", "", "Log storage root URL, https://ct.googleapis.com/rocketeer/")
	mysqlURI       = flag.String("mysql_uri", "", "URL of a MySQL database to clone the log into. The DB should contain only one log.")
	fetchBatchSize = flag.Uint("fetch_batch_size", 32, "The number of entries to fetch from the log in each request.")
	writeBatchSize = flag.Uint("write_batch_size", 32, "The number of leaves to write in each DB transaction.")
	workers        = flag.Uint("workers", 2, "The number of worker threads to run in parallel to fetch entries.")
)

func main() {
	flag.Parse()

	if !strings.HasSuffix(*logURL, "/") {
		glog.Exit("'log_url' must end with '/'")
	}
	if len(*mysqlURI) == 0 {
		glog.Exit("Missing required parameter 'mysql_uri'")
	}
	lu, err := url.Parse(*logURL)
	if err != nil {
		glog.Exitf("log_url is invalid: %v", err)
	}

	ctx := context.Background()
	db, err := logdb.NewDatabase(*mysqlURI)
	if err != nil {
		glog.Exitf("Failed to connect to database: %q", err)
	}
	// TODO(mhutchinson): Persist log URI and check here to prevent user-error writing different logs to same DB.
	head, err := db.Head()
	if err != nil {
		if err == logdb.ErrNoDataFound {
			glog.Infof("Failed to find head of database, assuming empty and starting from scratch: %v", err)
			head = -1
		} else {
			glog.Exitf("Failed to query for head of local log: %q", err)
		}
	}
	next := uint64(head + 1)

	fetcher := download.NewHTTPFetcher(lu)

	errChan := make(chan error)
	lc := make(chan []byte, *writeBatchSize*2)
	go download.Bulk(next, certLeafFetcher{fetcher}.Batch, *workers, *fetchBatchSize, lc, errChan)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	r := reporter{
		lastReported: next,
		lastReport:   time.Now(),
	}

	batch := make([][]byte, *writeBatchSize)
	bi := 0
	bs := next

	for {
		select {
		case err := <-errChan:
			glog.Exit(err)
		case <-ticker.C:
			r.report()
		case l := <-lc:
			workDone := r.trackWork(next)
			next++
			batch[bi] = l
			bi = (bi + 1) % int(*writeBatchSize)
			if bi == 0 {
				if err := db.WriteLeaves(ctx, bs, batch); err != nil {
					glog.Exitf("Failed to write to DB for batch starting at %d: %q", bs, err)
				}
				bs = next
			}
			workDone()
		}
	}
}

type reporter struct {
	lastReport   time.Time
	lastReported uint64
	lastWorked   uint64
	epochWorked  time.Duration
}

func (r *reporter) report() {
	elapsed := time.Since(r.lastReport)
	workRatio := r.epochWorked.Seconds() / elapsed.Seconds()

	rate := float64(r.lastWorked-r.lastReported) / elapsed.Seconds()
	glog.Infof("%.1f leaves/s, last leaf=%d, time working=%.1f%%", rate, r.lastReported, 100*workRatio)
	r.epochWorked = 0
	r.lastReported = r.lastWorked
	r.lastReport = time.Now()
}

func (r *reporter) trackWork(index uint64) func() {
	start := time.Now()
	r.lastWorked = index

	return func() {
		end := time.Now()
		r.epochWorked += end.Sub(start)
	}
}

// fetcher gets data paths. This allows impl to be swapped for tests.
type fetcher interface {
	// GetData gets the data at the given path, or returns an error.
	GetData(path string) ([]byte, error)
}

type certLeafFetcher struct {
	f fetcher
}

// Batch provides a mechanism to fetch a range of leaves.
// Enough leaves are fetched to fully fill `leaves`, or an error is returned.
// This implements batch.BatchFetch.
func (clf certLeafFetcher) Batch(start uint64, leaves [][]byte) error {
	// CT API gets [start, end] not [start, end).
	last := start + uint64(len(leaves)) - 1
	data, err := clf.f.GetData(fmt.Sprintf(getEntriesFormat, start, last))
	if err != nil {
		return fmt.Errorf("fetcher.GetData: %w", err)
	}
	var r getEntriesResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return fmt.Errorf("json.Unmarshal of %d bytes: %w", len(data), err)
	}
	if got, want := len(r.Leaves), len(leaves); got != want {
		return fmt.Errorf("wanted %d leaves but got %d", want, got)
	}
	for i, l := range r.Leaves {
		leaves[i] = l.Data
	}
	return nil
}

type getEntriesResponse struct {
	Leaves []leafInput `json:"entries"`
}

type leafInput struct {
	Data []byte `json:"leaf_input"`
}
