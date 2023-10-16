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

// sumdbclone is a one-shot tool for downloading entries from sum.golang.org.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/golang/glog"
	"github.com/google/trillian-examples/clone/internal/cloner"
	"github.com/google/trillian-examples/clone/internal/verify"
	"github.com/google/trillian-examples/clone/logdb"
	sdbclient "github.com/google/trillian-examples/sumdbaudit/client"
	"github.com/transparency-dev/merkle/rfc6962"

	_ "github.com/go-sql-driver/mysql"
)

var (
	vkey           = flag.String("vkey", "sum.golang.org+033de0ae+Ac4zctda0e5eza+HJyk9SxEdh+s3Ux18htTTAD8OuAn8", "The verification key for the log checkpoints")
	url            = flag.String("url", "https://sum.golang.org", "The base URL for the sumdb HTTP API.")
	mysqlURI       = flag.String("mysql_uri", "", "URL of a MySQL database to clone the log into. The DB should contain only one log.")
	writeBatchSize = flag.Uint("write_batch_size", 1024, "The number of leaves to write in each DB transaction.")
	workers        = flag.Uint("workers", 4, "The number of worker threads to run in parallel to fetch entries.")
	timeout        = flag.Duration("timeout", 10*time.Second, "Maximum time to wait for http connections to complete.")
	pollInterval   = flag.Duration("poll_interval", 0, "How often to poll the log for new checkpoints once all entries have been downloaded. Set to 0 to exit after download.")
)

const (
	tileHeight = 8
)

func main() {
	flag.Parse()

	if len(*mysqlURI) == 0 {
		glog.Exit("Missing required parameter 'mysql_uri'")
	}

	ctx := context.Background()
	db, err := logdb.NewDatabase(*mysqlURI)
	if err != nil {
		glog.Exitf("Failed to connect to database: %q", err)
	}

	client := sdbclient.NewSumDB(tileHeight, *vkey, *url, &http.Client{
		Timeout: *timeout,
	})
	cloneAndVerify(ctx, client, db)
	if *pollInterval == 0 {
		return
	}
	ticker := time.NewTicker(*pollInterval)
	for {
		select {
		case <-ticker.C:
			// Wait until the next tick.
		case <-ctx.Done():
			glog.Exit(ctx.Err())
		}
		cloneAndVerify(ctx, client, db)
	}
}

// cloneAndVerify verifies the downloaded leaves with the target checkpoint, and if it verifies, persists the checkpoint.
func cloneAndVerify(ctx context.Context, client *sdbclient.SumDBClient, db *logdb.Database) {
	targetCp, err := client.LatestCheckpoint()
	if err != nil {
		glog.Exitf("Failed to get latest checkpoint from log: %v", err)
	}
	glog.Infof("Target checkpoint is for tree size %d", targetCp.N)

	if err := clone(ctx, db, client, targetCp); err != nil {
		glog.Exitf("Failed to clone log: %v", err)
	}

	h := rfc6962.DefaultHasher
	lh := func(_ uint64, preimage []byte) []byte {
		return h.HashLeaf(preimage)
	}
	v := verify.NewLogVerifier(db, lh, h.HashChildren)
	root, crs, err := v.MerkleRoot(ctx, uint64(targetCp.N))
	if err != nil {
		glog.Exitf("Failed to compute root: %q", err)
	}
	if !bytes.Equal(targetCp.Hash[:], root) {
		glog.Exitf("Computed root %x != provided checkpoint %x for tree size %d", root, targetCp.Hash, targetCp.N)
	}
	glog.Infof("Got matching roots for tree size %d: %x", targetCp.N, root)
	if err := db.WriteCheckpoint(ctx, uint64(targetCp.N), targetCp.Raw, crs); err != nil {
		glog.Exitf("Failed to update database with new checkpoint: %v", err)
	}
}

func clone(ctx context.Context, db *logdb.Database, client *sdbclient.SumDBClient, targetCp *sdbclient.Checkpoint) error {
	fullTileSize := 1 << tileHeight
	cl := cloner.New(*workers, uint(fullTileSize), *writeBatchSize, db)

	next, err := cl.Next()
	if err != nil {
		return fmt.Errorf("couldn't determine first leaf to fetch: %v", err)
	}
	if next >= uint64(targetCp.N) {
		glog.Infof("No work to do. Local tree size = %d, latest log tree size = %d", next, targetCp.N)
		return nil
	}

	// batchFetch gets full or partial tiles depending on the number of leaves requested.
	// start must always be the first index within a tile or it is being used wrong.
	batchFetch := func(start uint64, leaves [][]byte) error {
		if start%uint64(fullTileSize) > 0 {
			return backoff.Permanent(fmt.Errorf("%d is not the first leaf in a tile", start))
		}
		offset := int(start >> tileHeight)

		var got [][]byte
		var err error
		if len(leaves) == fullTileSize {
			got, err = client.FullLeavesAtOffset(offset)
		} else {
			got, err = client.PartialLeavesAtOffset(offset, len(leaves))
		}
		if err != nil {
			return fmt.Errorf("failed to get leaves at offset %d: %v", offset, err)
		}
		copy(leaves, got)
		return nil
	}

	// Download any remainder of a previous partial tile before calling Clone,
	// which is optimized for chunks perfectly aligning with tiles.
	if rem := next % uint64(fullTileSize); rem > 0 {
		tileStart := next - rem
		needed := fullTileSize
		if d := targetCp.N - int64(tileStart); int(d) < needed {
			needed = int(d)
		}
		leaves := make([][]byte, needed)
		glog.Infof("Next=%d does not align with tile boundary; prefetching tile [%d, %d)", next, tileStart, tileStart+uint64(len(leaves)))
		if err := batchFetch(tileStart, leaves); err != nil {
			return err
		}
		if err := db.WriteLeaves(ctx, next, leaves[rem:]); err != nil {
			return fmt.Errorf("failed to write to DB for batch starting at %d: %q", next, err)
		}
	}

	// The database must now contain only complete tiles, or else be matched with
	// the targetCp. Either way, the preconditions for the cloner configuration are met.
	if err := cl.Clone(ctx, uint64(targetCp.N), batchFetch); err != nil {
		return fmt.Errorf("failed to clone log: %v", err)
	}
	return nil
}
