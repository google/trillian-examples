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

// Package main provides a command line tool for integrating sequenced
// entries into a serverless log.
package main

import (
	"flag"
	"io/ioutil"
	"path/filepath"

	"github.com/google/trillian-examples/serverless/internal/storage/fs"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/serverless/internal/log"
	"github.com/google/trillian/merkle/rfc6962/hasher"
)

var (
	storageDir = flag.String("storage_dir", "", "Root directory to store log data.")
	entries    = flag.String("entries", "", "File path glob of entries to add to the log.")
	create     = flag.Bool("create", false, "Set when creating a new log to initialise the structure.")
)

func main() {
	flag.Parse()

	h := hasher.DefaultHasher
	// init storage
	var st *fs.FS
	var err error
	if *create {
		st, err = fs.Create(*storageDir, h.EmptyRoot())
	} else {
		st, err = fs.New(*storageDir)
	}
	if err != nil {
		glog.Exitf("Failed to initialise storage: %q", err)
	}

	// sequence entries
	toAdd, err := filepath.Glob(*entries)
	if err != nil {
		glog.Exitf("Failed to glob entries %q: %q", *entries, err)
	}

	entries := make(chan []byte, 100)
	go func() {
		for _, fp := range toAdd {
			entry, err := ioutil.ReadFile(fp)
			if err != nil {
				glog.Exitf("Failed to read entry file %q: %q", fp, err)
			}
			entries <- entry
		}
		close(entries)
	}()

	if err := log.Sequence(st, h, entries); err != nil {
		glog.Exitf("Failed to sequence entries: %q", err)
	}
}
