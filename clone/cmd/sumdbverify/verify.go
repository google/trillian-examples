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

// verify checks that a cloned SumDB log does not contain any conflicting entries.
package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/google/trillian-examples/clone/logdb"
	"k8s.io/klog/v2"

	_ "github.com/go-sql-driver/mysql"
)

var (
	mysqlURI = flag.String("mysql_uri", "", "URI of the MySQL database containing the log.")
)

type dataSource interface {
	GetLatestCheckpoint(ctx context.Context) (size uint64, checkpoint []byte, compactRange [][]byte, err error)
	StreamLeaves(ctx context.Context, start, end uint64, out chan<- logdb.StreamResult)
}

func main() {
	flag.Parse()
	ctx := context.Background()

	if len(*mysqlURI) == 0 {
		klog.Exit("Missing required parameter 'mysql_uri'")
	}
	db, err := logdb.NewDatabase(*mysqlURI)
	if err != nil {
		klog.Exitf("Failed to connect to database: %q", err)
	}

	size, err := verifyLeaves(ctx, db)
	if err != nil {
		klog.Exitf("Failed verification: %v", err)
	}
	klog.Infof("No conflicting hashes found after verifying %d leaves", size)
}

func verifyLeaves(ctx context.Context, db dataSource) (uint64, error) {
	leaves := make(chan logdb.StreamResult, 1)
	size, _, _, err := db.GetLatestCheckpoint(ctx)
	if err != nil {
		return 0, fmt.Errorf("GetLatestCheckpoint(): %v", err)
	}
	go db.StreamLeaves(ctx, 0, size, leaves)

	modVerToHashes := make(map[string]string)

	var index uint64
	for leaf := range leaves {
		if leaf.Err != nil {
			return 0, fmt.Errorf("failed to get leaves from DB: %w", leaf.Err)
		}
		data := leaf.Leaf

		// Example leaf:
		// golang.org/x/text v0.3.0 h1:g61tztE5qeGQ89tm6NTjjM9VPIm088od1l6aSorWRWg=
		// golang.org/x/text v0.3.0/go.mod h1:NqM8EUOU14njkJ3fqMW+pc6Ldnwhi/IjpwHt7yyuwOQ=
		//
		lines := strings.Split(string(data), "\n")
		line0 := strings.Split(lines[0], " ")
		module, version, repoHash := line0[0], line0[1], line0[2]

		line1 := strings.Split(lines[1], " ")
		if got, want := line1[0], module; got != want {
			return 0, fmt.Errorf("mismatched module names at %d: (%s, %s)", index, got, want)
		}
		if got, want := line1[1][:len(version)], version; got != want {
			return 0, fmt.Errorf("mismatched version names at %d: (%s, %s)", index, got, want)
		}
		modHash := line1[2]

		modVer := fmt.Sprintf("%s %s", module, version)
		hashes := fmt.Sprintf("%s %s", repoHash, modHash)

		if existing, found := modVerToHashes[modVer]; found {
			klog.V(1).Infof("Found existing hash for %q", modVer)
			if existing != hashes {
				return 0, fmt.Errorf("module and version %q has conflicting hashes!\n%q != %q", modVer, existing, hashes)
			}
		}
		modVerToHashes[modVer] = hashes
		index++
	}
	return index, nil
}
