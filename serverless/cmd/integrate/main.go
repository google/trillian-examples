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
	"io/ioutil"
	"net/url"
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
	storageDir  = flag.String("storage_dir", "", "Root directory to store log data.")
	initialise  = flag.Bool("initialise", false, "Set when creating a new log to initialise the structure.")
	pubKeyFile  = flag.String("public_key", "", "Location of public key file.")
	privKeyFile = flag.String("private_key", "", "Location of private key file.")
)

func main() {
	flag.Parse()
	h := hasher.DefaultHasher

	// Read log public key from file or environment variable
	var pubKey string
	if len(*pubKeyFile) > 0 {
		pubKeyURL, err := url.Parse(*pubKeyFile)
		if err != nil {
			glog.Exitf("failed to parse public_key path: %q", err)
		}
		k, err := ioutil.ReadFile(pubKeyURL.Path)
		if err != nil {
			glog.Exitf("failed to read public_key file: %q", err)
		}
		pubKey = string(k)
	} else {
		pubKey = os.Getenv("SERVERLESS_LOG_PUBLIC_KEY")
		if len(pubKey) == 0 {
			glog.Exit(`supply public key file path using --public_key 
				or set SERVERLESS_LOG_PUBLIC_KEY environment variable`)
		}
	}

	// Read log private key from file or environment variable
	var privKey string
	if len(*privKeyFile) > 0 {
		privKeyURL, err := url.Parse(*privKeyFile)
		if err != nil {
			glog.Exitf("failed to parse private_key path: %q", err)
		}
		k, _ := ioutil.ReadFile(privKeyURL.Path)
		privKey = string(k)
		if err != nil {
			glog.Exitf("failed to read private_key file: %q", err)
		}
	} else {
		privKey = os.Getenv("SERVERLESS_LOG_PRIVATE_KEY")
		if len(privKey) == 0 {
			glog.Exit(`supply private key file path using --private_key 
			or set SERVERLESS_LOG_PUBLIC_KEY environment variable`)
		}
	}

	var cpNote note.Note
	s, err := note.NewSigner(privKey)
	if err != nil {
		glog.Exitf("failed to instantiate signer: %q", err)
	}
	if *initialise {
		st, err := fs.Create(*storageDir, h.EmptyRoot())
		if err != nil {
			glog.Exitf("failed to create log: %q", err)
		}
		cp := st.Checkpoint()
		cp.Ecosystem = api.CheckpointHeaderV0
		cpNote.Text = string(cp.Marshal())
		cpNoteSigned, err := note.Sign(&cpNote, s)
		if err != nil {
			glog.Exitf("failed to sign Checkpoint: %q", err)
		}
		if err := st.WriteCheckpoint(cpNoteSigned); err != nil {
			glog.Exitf("failed to store new log checkpoint: %q", err)
		}
		os.Exit(0)
	}

	// init storage
	cpRaw, err := fs.ReadCheckpoint(*storageDir)
	if err != nil {
		glog.Exitf("failed to read log checkpoint: %q", err)
	}

	// Check signatures
	v, err := note.NewVerifier(pubKey)
	if err != nil {
		glog.Exitf("Failed to instantiate Verifier: %q", err)
	}
	verifiers := note.VerifierList(v)
	vCp, err := note.Open(cpRaw, verifiers)
	if err != nil {
		glog.Exitf("failed to open Checkpoint: %q", err)
	}

	var cp fmtlog.Checkpoint
	if _, err := cp.Unmarshal([]byte(vCp.Text)); err != nil {
		glog.Exitf("failed to unmarshal checkpoint: %q", err)
	}
	st, err := fs.Load(*storageDir, &cp)
	if err != nil {
		glog.Exitf("failed to load storage: %q", err)
	}

	// Integrate new entries
	newCp, err := log.Integrate(st, h)
	if err != nil {
		glog.Exitf("failed to integrate: %q", err)
	}
	if newCp == nil {
		glog.Exit("nothing to integrate")
	}

	newCp.Ecosystem = api.CheckpointHeaderV0

	// Sign the note
	cpNote.Text = string(newCp.Marshal())
	cpNoteSigned, err := note.Sign(&cpNote, s)
	if err != nil {
		glog.Exitf("failed to sign Checkpoint: %q", err)
	}

	// Persist new log checkpoint.
	if err := st.WriteCheckpoint(cpNoteSigned); err != nil {
		glog.Exitf("failed to store new log checkpoint: %q", err)
	}
}
