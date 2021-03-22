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

// Package main is a read-only client command for interacting with serverless logs.
package main

import (
	"flag"
	"fmt"
	"io/ioutil"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/serverless/api"
	"github.com/google/trillian-examples/serverless/internal/client"
	"github.com/google/trillian-examples/serverless/internal/storage/fs"
	"github.com/google/trillian/merkle/logverifier"
	"github.com/google/trillian/merkle/rfc6962/hasher"
)

var (
	storageDir   = flag.String("storage_dir", "", "Root directory to store log data.")
	fromIndex    = flag.Uint64("from_index", 0, "Index for inclusion proof")
	forEntryPath = flag.String("for_entry", "", "Path to entry for inclusion proof")
)

func main() {
	flag.Parse()
	// init storage
	st, err := fs.New(*storageDir)
	if err != nil {
		glog.Exitf("Failed to load storage: %q", err)
	}

	args := flag.Args()
	if len(args) == 0 {
		glog.Exit("Please specify a command from [inclusion]")
	}
	switch args[0] {
	case "inclusion":
		err = inclusionProof(st.LogState(), st.GetTile, args[1:])
	}
	if err != nil {
		glog.Exitf("Command %q failed: %q", args[0], err)
	}
}

func inclusionProof(state api.LogState, f client.GetTileFunc, args []string) error {
	entry, err := ioutil.ReadFile(*forEntryPath)
	if err != nil {
		return fmt.Errorf("failed to read entry from %q: %q", *forEntryPath, err)
	}
	lh := hasher.DefaultHasher.HashLeaf(entry)

	proof, err := client.InclusionProof(*fromIndex, state.Size, f)
	if err != nil {
		return fmt.Errorf("failed to get inclusion proof: %w", err)
	}

	lv := logverifier.New(hasher.DefaultHasher)
	if err := lv.VerifyInclusionProof(int64(*fromIndex), int64(state.Size), proof, state.RootHash, lh); err != nil {
		return fmt.Errorf("failed to verify inclusion proof: %q", err)
	}

	glog.Info("Inclusion verified.")
	return nil
}
