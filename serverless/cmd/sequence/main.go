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
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/google/trillian-examples/serverless/api"
	"github.com/google/trillian-examples/serverless/internal/storage"
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
		r, _ := api.LogState{}.MarshalText()
		if err := st.WriteLogState(r); err != nil {
			glog.Exitf("Failed to create empty log state: %q", err)
		}

	} else {
		var stateRaw []byte
		stateRaw, err = fs.ReadLogState(*storageDir)
		if err != nil {
			glog.Exitf("Failed to read log state: %q", err)
		}
		var state api.LogState
		if err := state.UnmarshalText(stateRaw); err != nil {
			glog.Exitf("Failed to unmarshal state: %q", err)
		}
		st, err = fs.Load(*storageDir, &state)
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
		dupe := false
		seq, err := st.Sequence(lh, entry.b)
		if err != nil {
			if errors.Is(err, storage.ErrDupeLeaf) {
				dupe = true
			} else {
				glog.Exitf("failed to sequence %q: %q", entry.name, err)
			}
		}
		l := fmt.Sprintf("%d: %v", seq, entry.name)
		if dupe {
			l += " (dupe)"
		}
		glog.Info(l)
	}
}
