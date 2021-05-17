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

// Package main provides a command line tool for sequencing entries in
// a serverless log.
package main

import (
	"flag"
	"os"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/serverless/api"
	"github.com/google/trillian-examples/serverless/internal/log"
	"github.com/google/trillian-examples/serverless/internal/storage/fs"
	"github.com/google/trillian/merkle/rfc6962/hasher"
	"golang.org/x/mod/sumdb/note"

	fmtlog "github.com/google/trillian-examples/formats/log"
)

var (
	storageDir = flag.String("storage_dir", "", "Root directory to store log data.")
	initial    = flag.Bool("initial", false, "Set when creating a new log to initialise the structure.")
)

func main() {
	flag.Parse()
	h := hasher.DefaultHasher

	privKey := os.Getenv("SERVERLESS_LOG_PRIVATE_KEY")
	pubKey := os.Getenv("SERVERLESS_LOG_PUBLIC_KEY")

	var cpNote note.Note
	s, err := note.NewSigner(privKey)
	if err != nil {
		glog.Exitf("Failed to instantiate signer: %q", err)
	}
	if *initial {
		st, err := fs.Create(*storageDir, h.EmptyRoot())
		if err != nil {
			glog.Exitf("Failed to create log: %q", err)
		}
		cp := st.Checkpoint()
		cp.Ecosystem = api.CheckpointHeaderV0
		cpNote.Text = string(cp.Marshal())
		cpNoteSigned, err := note.Sign(&cpNote, s)
		if err != nil {
			glog.Exitf("Failed to sign Checkpoint: %q", err)
		}
		if err := st.WriteCheckpoint(cpNoteSigned); err != nil {
			glog.Exitf("Failed to store new log checkpoint: %q", err)
		}

	}

	// init storage
	cpRaw, err := fs.ReadCheckpoint(*storageDir)
	if err != nil {
		glog.Exitf("Failed to read log checkpoint: %q", err)
	}

	// Check signatures
	v, _ := note.NewVerifier(pubKey)
	verifiers := note.VerifierList(v)
	vCp, err := note.Open(cpRaw, verifiers)
	if err != nil {
		glog.Exitf("Failed to open Checkpoint: %q", err)
	}

	var cp fmtlog.Checkpoint
	if _, err := cp.Unmarshal([]byte(vCp.Text)); err != nil {
		glog.Exitf("Failed to unmarshal checkpoint: %q", err)
	}
	st, err := fs.Load(*storageDir, &cp)
	if err != nil {
		glog.Exitf("Failed to load storage: %q", err)
	}

	// Integrate new entries
	newCp, err := log.Integrate(st, h)
	if err != nil {
		glog.Exitf("Failed to integrate: %q", err)
	}
	if newCp == nil {
		glog.Exit("Nothing to integrate")
	}

	newCp.Ecosystem = api.CheckpointHeaderV0

	// Sign the note
	cpNote.Text = string(newCp.Marshal())
	cpNoteSigned, err := note.Sign(&cpNote, s)
	if err != nil {
		glog.Exitf("Failed to sign Checkpoint: %q", err)
	}

	// Persist new log checkpoint.
	if err := st.WriteCheckpoint(cpNoteSigned); err != nil {
		glog.Exitf("Failed to store new log checkpoint: %q", err)
	}
}
