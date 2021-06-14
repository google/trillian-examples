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
	"os"
	"path/filepath"

	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/serverless/internal/storage"
	"github.com/google/trillian-examples/serverless/internal/storage/fs"
	"golang.org/x/mod/sumdb/note"

	"github.com/golang/glog"
	"github.com/google/trillian/merkle/rfc6962/hasher"
)

var (
	storageDir = flag.String("storage_dir", "", "Root directory to store log data.")
	entries    = flag.String("entries", "", "File path glob of entries to add to the log.")
	pubKeyFile = flag.String("public_key", "", "Location of public key file. If unset, uses the contents of the SERVERLESS_LOG_PUBLIC_KEY environment variable.")
)

func main() {
	flag.Parse()

	// Read log public key from file or environment variable
	var pubKey string
	if len(*pubKeyFile) > 0 {
		k, err := ioutil.ReadFile(*pubKeyFile)
		if err != nil {
			glog.Exitf("failed to read public_key file: %q", err)
		}
		pubKey = string(k)
	} else {
		pubKey = os.Getenv("SERVERLESS_LOG_PUBLIC_KEY")
		if len(pubKey) == 0 {
			glog.Exit("supply public key file path using --public_key or set SERVERLESS_LOG_PUBLIC_KEY environment variable")
		}
	}

	toAdd, err := filepath.Glob(*entries)
	if err != nil {
		glog.Exitf("Failed to glob entries %q: %q", *entries, err)
	}
	if len(toAdd) == 0 {
		glog.Exit("Sequence must be run with at least one valid entry")
	}

	h := hasher.DefaultHasher
	// init storage

	cpRaw, err := fs.ReadCheckpoint(*storageDir)
	if err != nil {
		glog.Exitf("Failed to read log checkpoint: %q", err)
	}

	// Check signatures
	v, err := note.NewVerifier(pubKey)
	if err != nil {
		glog.Exitf("Failed to instantiate Verifier: %q", err)
	}
	vCp, err := note.Open(cpRaw, note.VerifierList(v))
	if err != nil {
		glog.Exitf("Failed to open Checkpoint: %q", err)
	}

	var cp log.Checkpoint
	if _, err := cp.Unmarshal([]byte(vCp.Text)); err != nil {
		glog.Exitf("Failed to unmarshal checkpoint: %q", err)
	}
	st, err := fs.Load(*storageDir, &cp)
	if err != nil {
		glog.Exitf("Failed to load storage: %q", err)
	}

	// sequence entries

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
