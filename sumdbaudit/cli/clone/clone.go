// Copyright 2020 Google LLC
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

package main

import (
	"context"

	"flag"

	"github.com/golang/glog"

	"github.com/google/trillian-examples/sumdbaudit/audit"
	_ "github.com/mattn/go-sqlite3"
)

var (
	height = flag.Int("h", 8, "tile height")
	vkey   = flag.String("k", "sum.golang.org+033de0ae+Ac4zctda0e5eza+HJyk9SxEdh+s3Ux18htTTAD8OuAn8", "key")
	db     = flag.String("db", "./sum.db", "database file location (will be created if it doesn't exist)")
	extraV = flag.Bool("x", false, "performs additional checks on each tile hashes")
	force  = flag.Bool("f", false, "forces the auditor to run even if no new data is available")
)

// Clones the leaves of the SumDB into the local database and verifies the result.
// This does not perform any checks on the leaf data to look for inconsistent claims.
// If this returns successfully, it means that all leaf data in the DB matches that
// contained in the SumDB.
func main() {
	ctx := context.Background()
	flag.Parse()

	db, err := audit.NewDatabase(*db)
	if err != nil {
		glog.Exitf("failed to open DB: %v", err)
	}
	err = db.Init()
	if err != nil {
		glog.Exitf("failed to init DB: %v", err)
	}

	sumDB := audit.NewSumDB(*height, *vkey)
	checkpoint, err := sumDB.LatestCheckpoint()
	if err != nil {
		glog.Exitf("failed to get latest checkpoint: %s", err)
	}

	glog.Infof("Got SumDB checkpoint for %d entries. Downloading...", checkpoint.N)
	s := audit.NewService(db, sumDB, *height)
	if !*force {
		golden := s.GoldenCheckpoint(ctx)
		if golden != nil && golden.N >= checkpoint.N {
			glog.Infof("nothing to do: latest SumDB size is %d and local size is %d", checkpoint.N, golden.N)
			return
		}
	}

	if err := s.CloneLeafTiles(ctx, checkpoint); err != nil {
		glog.Exitf("failed to update leaves: %v", err)
	}
	glog.Infof("Updated leaves to latest checkpoint (tree size %d). Calculating hashes...", checkpoint.N)

	if err := s.HashTiles(ctx, checkpoint); err != nil {
		glog.Exitf("HashTiles: %v", err)
	}
	glog.Infof("Hashes updated successfully. Checking consistency with previous checkpoint...")
	if err := s.CheckConsistency(ctx); err != nil {
		glog.Exitf("CheckConsistency: %v", err)
	}
	glog.Infof("Log consistent. Checking root hash with remote...")
	if err := s.CheckRootHash(ctx, checkpoint); err != nil {
		glog.Exitf("CheckRootHash: %v", err)
	}
	glog.Infof("Cloned successfully. Tree size is %d, hash is %x (%s). Processing data...", checkpoint.N, checkpoint.Hash[:], checkpoint.Hash)

	if err := s.ProcessMetadata(ctx, checkpoint); err != nil {
		glog.Exitf("ProcessMetadata: %v", err)
	}
	glog.Infof("Leaf data processed. Checking for duplicates...")

	dups, err := db.Duplicates()
	if err != nil {
		glog.Exitf("Duplicates: %v", err)
	}
	if len(dups) > 0 {
		for _, d := range dups {
			glog.Errorf("%d duplicates found for %s %s", d.Count, d.Module, d.Version)
		}
		glog.Exitf("Duplicate entries is a critical error")
	}
	glog.Info("No duplicates found")

	if *extraV {
		glog.Infof("Performing extra validation on tiles...")
		if err := s.VerifyTiles(ctx, checkpoint); err != nil {
			glog.Exitf("VerifyTiles: %v", err)
		}
		glog.Infof("Tile verificaton passed")
	}
}
