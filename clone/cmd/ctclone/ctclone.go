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
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"strings"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/clone/internal/cloner"
	"github.com/google/trillian-examples/clone/internal/download"
	"github.com/google/trillian-examples/clone/internal/verify"
	"github.com/google/trillian-examples/clone/logdb"
	"github.com/transparency-dev/merkle/rfc6962"

	_ "github.com/go-sql-driver/mysql"
)

var (
	logURL         = flag.String("log_url", "", "Log storage root URL, e.g. https://ct.googleapis.com/rocketeer/")
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

	// Get the latest checkpoint from the log we are cloning: we will download all the leaves this commits to.
	fetcher := ctFetcher{download.NewHTTPFetcher(lu)}
	targetCp, err := fetcher.latestCheckpoint()
	if err != nil {
		glog.Exitf("Failed to get latest checkpoint from log: %v", err)
	}

	cl := cloner.New(*workers, *fetchBatchSize, *writeBatchSize, db)
	if err := cl.Clone(ctx, targetCp.TreeSize, fetcher.Batch); err != nil {
		glog.Exitf("Failed to clone log: %v", err)
	}

	glog.Info("Verifying leaves")
	// Verify the downloaded leaves with the target checkpoint, and if it verifies, persist the checkpoint.
	// TODO(mhutchinson): Verify in parallel with downloading.
	h := rfc6962.DefaultHasher
	lh := func(_ uint64, preimage []byte) []byte {
		return h.HashLeaf(preimage)
	}
	v := verify.NewLogVerifier(db, lh, h.HashChildren)
	root, crs, err := v.MerkleRoot(ctx, targetCp.TreeSize)
	if err != nil {
		glog.Exitf("Failed to compute root: %q", err)
	}
	if !bytes.Equal(targetCp.RootHash, root) {
		glog.Exitf("Computed root %x != provided checkpoint %x for tree size %d", root, targetCp.RootHash, targetCp.TreeSize)
	}
	glog.Infof("Got matching roots for tree size %d: %x", targetCp.TreeSize, root)
	if err := db.WriteCheckpoint(ctx, targetCp.TreeSize, targetCp.raw, crs); err != nil {
		glog.Exitf("Failed to update database with new checkpoint: %v", err)
	}
}

// fetcher gets data paths. This allows impl to be swapped for tests.
type fetcher interface {
	// GetData gets the data at the given path, or returns an error.
	GetData(path string) ([]byte, error)
}

type ctFetcher struct {
	f fetcher
}

// Batch provides a mechanism to fetch a range of leaves.
// Enough leaves are fetched to fully fill `leaves`, or an error is returned.
// This implements batch.BatchFetch.
func (cf ctFetcher) Batch(start uint64, leaves [][]byte) error {
	// CT API gets [start, end] not [start, end).
	last := start + uint64(len(leaves)) - 1
	data, err := cf.f.GetData(fmt.Sprintf("ct/v1/get-entries?start=%d&end=%d", start, last))
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

func (cf ctFetcher) latestCheckpoint() (CTCheckpointResponse, error) {
	r := CTCheckpointResponse{}
	cpbs, err := cf.f.GetData("ct/v1/get-sth")
	if err != nil {
		return r, fmt.Errorf("failed to find latest log checkpoint: %v", err)
	}
	if err := json.Unmarshal(cpbs, &r); err != nil {
		return r, fmt.Errorf("failed to parse checkpoint: %v", err)
	}
	r.raw = cpbs
	return r, nil
}

type getEntriesResponse struct {
	Leaves []leafInput `json:"entries"`
}

type leafInput struct {
	Data []byte `json:"leaf_input"`
}

// CTCheckpointResponse mirrors the RFC6962 STH format for `get-sth` to allow the
// data to be easy unmarshalled from the JSON response.
// TODO(mhutchinson): this was copied from ctverify. Deduplicate.
type CTCheckpointResponse struct {
	TreeSize  uint64 `json:"tree_size"`
	Timestamp uint64 `json:"timestamp"`
	RootHash  []byte `json:"sha256_root_hash"`
	Sig       []byte `json:"tree_head_signature"`

	raw []byte
}
