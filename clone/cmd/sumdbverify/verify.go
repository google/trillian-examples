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

// verify checks that a cloned SumDB log does not contain any conflicting entries.
package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/clone/logdb"

	_ "github.com/go-sql-driver/mysql"
)

var (
	mysqlURI = flag.String("mysql_uri", "", "URL of the MySQL database containing the log.")
)

func main() {
	flag.Parse()
	ctx := context.Background()

	if len(*mysqlURI) == 0 {
		glog.Exit("Missing required parameter 'mysql_uri'")
	}
	db, err := logdb.NewDatabase(*mysqlURI)
	if err != nil {
		glog.Exitf("Failed to connect to database: %q", err)
	}

	leaves := make(chan logdb.StreamResult, 1)
	size, _, _, err := db.GetLatestCheckpoint(ctx)
	if err != nil {
		glog.Exitf("GetLatestCheckpoint(): %v", err)
	}
	go db.StreamLeaves(ctx, 0, size, leaves)

	modVerToHashes := make(map[string]string)

	var index uint64
	for leaf := range leaves {
		if leaf.Err != nil {
			glog.Exitf("Failed to get leaves from DB: %w", leaf.Err)
		}
		data := leaf.Leaf

		// Example leaf:
		// golang.org/x/text v0.3.0 h1:g61tztE5qeGQ89tm6NTjjM9VPIm088od1l6aSorWRWg=
		// golang.org/x/text v0.3.0/go.mod h1:NqM8EUOU14njkJ3fqMW+pc6Ldnwhi/IjpwHt7yyuwOQ=
		//

		lines := strings.Split(string(data), "\n")
		tokens := strings.Split(lines[0], " ")
		module, version, repoHash := tokens[0], tokens[1], tokens[2]

		tokens = strings.Split(lines[1], " ")
		if got, want := tokens[0], module; got != want {
			glog.Exitf("Mismatched module names at %d: (%s, %s)", index, got, want)
		}
		if got, want := tokens[1][:len(version)], version; got != want {
			glog.Exitf("Mismatched version names at %d: (%s, %s)", index, got, want)
		}
		modHash := tokens[2]

		modVer := fmt.Sprintf("%s %s", module, version)
		hashes := fmt.Sprintf("%s %s", repoHash, modHash)

		if existing, found := modVerToHashes[modVer]; found {
			glog.V(1).Infof("Found existing hash for %q", modVer)
			if existing != hashes {
				glog.Exitf("Module and version %q has conflicting hashes!\n%q != %q", existing, hashes)
			}
		}
		modVerToHashes[modVer] = hashes
		index++
	}
	glog.Infof("No conflicting hashes found after verifying %d leaves", index)
}
