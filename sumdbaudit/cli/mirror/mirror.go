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
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/sumdbaudit/audit"
)

var (
	height   = flag.Int("h", 8, "tile height")
	vkey     = flag.String("k", "sum.golang.org+033de0ae+Ac4zctda0e5eza+HJyk9SxEdh+s3Ux18htTTAD8OuAn8", "key")
	unpack   = flag.Bool("unpack", false, "if provided then the leafMetadata table will be populated by parsing the raw leaf data")
	interval = flag.Duration("interval", 5*time.Minute, "how long to wait between runs")
)

// Runs a service which continually keeps a local database up to date with a verified
// clone of the Go SumDB.
func main() {
	ctx := context.Background()
	flag.Parse()

	db, err := audit.NewDatabaseFromFlags()
	if err != nil {
		glog.Exitf("Failed to open DB: %v", err)
	}
	err = db.Init()
	if err != nil {
		glog.Exitf("Failed to init DB: %v", err)
	}
	sumDB := audit.NewSumDB(*height, *vkey)
	s := audit.NewService(db, sumDB, *height)

	for {
		head, err := sumDB.LatestCheckpoint()
		if err != nil {
			glog.Exitf("Failed to get latest checkpoint: %s", err)
		}
		golden := s.GoldenCheckpoint(ctx)
		if golden != nil && golden.N >= head.N {
			glog.Infof("Nothing to do: latest SumDB size is %d and local size is %d", head.N, golden.N)
		} else {
			glog.Infof("Syncing to latest SumDB size %d", head.N)
			if err := s.Sync(ctx, head); err != nil {
				glog.Exitf("Sync: %v", err)
			}
			if *unpack {
				glog.V(1).Infof("Processing metadata")
				if err := s.ProcessMetadata(ctx, head); err != nil {
					glog.Exitf("ProcessMetadata: %v", err)
				}
			}
		}

		glog.V(1).Infof("Sleeping for %v", *interval)
		time.Sleep(*interval)
	}
}
