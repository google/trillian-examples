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
	"os"
	"path/filepath"

	"github.com/google/trillian-examples/serverless/internal/storage/fs"

	"github.com/golang/glog"
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
	var st *fs.Storage
	var err error
	if *create {
		st, err = fs.Create(*storageDir, h.EmptyRoot())
	} else {
		st, err = fs.Load(*storageDir)
	}
	if err != nil {
		glog.Exitf("Failed to initialise storage: %q", err)
	}

	// sequence entries
	toAdd, err := filepath.Glob(*entries)
	if err != nil {
		glog.Exitf("Failed to glob entries %q: %q", *entries, err)
	}

	// entryInfo binds the actual bytes to be added as a leaf with a
	// user-recognisable name for the source of those bytes.
	// The name is only used below in order to inform the user of the
	// sequence numbers assigned to the data from the provided input files.
	type entryInfo struct {
		name string
		b    []byte
	}
	entries := make(chan entryInfo, 100)
	go func() {
		for _, fp := range toAdd {
			b, err := ioutil.ReadFile(fp)
			if err != nil {
				glog.Exitf("Failed to read entry file %q: %q", fp, err)
			}
			entries <- entryInfo{name: fp, b: b}
		}
		close(entries)
	}()

	for entry := range entries {
		// ask storage to sequence
		lh := h.HashLeaf(entry.b)
		seq, err := st.Sequence(lh, entry.b)
		if err != nil {
			if os.IsExist(err) {
				glog.Infof("Skipping dupe entry %q with hash 0x%x", entry.name, lh)
			} else {
				glog.Fatalf("failed to sequence %q: %q", lh, err)
			}
		}
		glog.Infof("%d: %v", seq, entry.name)
	}
}
