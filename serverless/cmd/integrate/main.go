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
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/serverless/internal/storage/fs"
	"github.com/google/trillian-examples/serverless/pkg/log"
	"github.com/transparency-dev/merkle/rfc6962"
	"golang.org/x/mod/sumdb/note"

	fmtlog "github.com/google/trillian-examples/formats/log"
)

var (
	storageDir  = flag.String("storage_dir", "", "Root directory to store log data.")
	initialise  = flag.Bool("initialise", false, "Set when creating a new log to initialise the structure.")
	pubKeyFile  = flag.String("public_key", "", "Location of public key file. If unset, uses the contents of the SERVERLESS_LOG_PUBLIC_KEY environment variable.")
	privKeyFile = flag.String("private_key", "", "Location of private key file. If unset, uses the contents of the SERVERLESS_LOG_PRIVATE_KEY environment variable.")
	origin      = flag.String("origin", "", "Log origin string to use in produced checkpoint.")
)

func main() {
	flag.Parse()
	ctx := context.Background()

	if len(*origin) == 0 {
		glog.Exitf("Please set --origin flag to log identifier.")
	}

	h := rfc6962.DefaultHasher
	// Read log public key from file or environment variable
	var pubKey string
	var err error
	if len(*pubKeyFile) > 0 {
		pubKey, err = getKeyFile(*pubKeyFile)
		if err != nil {
			glog.Exitf("Unable to get public key: %q", err)
		}
	} else {
		pubKey = os.Getenv("SERVERLESS_LOG_PUBLIC_KEY")
		if len(pubKey) == 0 {
			glog.Exit("Supply public key file path using --public_key or set SERVERLESS_LOG_PUBLIC_KEY environment variable")
		}
	}
	// Read log private key from file or environment variable
	var privKey string
	if len(*privKeyFile) > 0 {
		privKey, err = getKeyFile(*privKeyFile)
		if err != nil {
			glog.Exitf("Unable to get private key: %q", err)
		}
	} else {
		privKey = os.Getenv("SERVERLESS_LOG_PRIVATE_KEY")
		if len(privKey) == 0 {
			glog.Exit("Supply private key file path using --private_key or set SERVERLESS_LOG_PUBLIC_KEY environment variable")
		}
	}

	var cpNote note.Note
	s, err := note.NewSigner(privKey)
	if err != nil {
		glog.Exitf("Failed to instantiate signer: %q", err)
	}

	if *initialise {
		st, err := fs.Create(*storageDir)
		if err != nil {
			glog.Exitf("Failed to create log: %q", err)
		}
		cp := fmtlog.Checkpoint{
			Hash: h.EmptyRoot(),
		}
		if err := signAndWrite(ctx, &cp, cpNote, s, st); err != nil {
			glog.Exitf("Failed to sign: %q", err)
		}
		os.Exit(0)
	}

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
	cp, _, _, err := fmtlog.ParseCheckpoint(cpRaw, *origin, v)
	if err != nil {
		glog.Exitf("Failed to open Checkpoint: %q", err)
	}
	st, err := fs.Load(*storageDir, cp.Size)
	if err != nil {
		glog.Exitf("Failed to load storage: %q", err)
	}

	// Integrate new entries
	newCp, err := log.Integrate(ctx, *cp, st, h)
	if err != nil {
		glog.Exitf("Failed to integrate: %q", err)
	}
	if newCp == nil {
		glog.Exit("Nothing to integrate")
	}

	err = signAndWrite(ctx, newCp, cpNote, s, st)
	if err != nil {
		glog.Exitf("Failed to sign: %q", err)
	}
}

func getKeyFile(path string) (string, error) {
	k, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read key file: %w", err)
	}
	return string(k), nil
}

func signAndWrite(ctx context.Context, cp *fmtlog.Checkpoint, cpNote note.Note, s note.Signer, st *fs.Storage) error {
	cp.Origin = *origin
	cpNote.Text = string(cp.Marshal())
	cpNoteSigned, err := note.Sign(&cpNote, s)
	if err != nil {
		return fmt.Errorf("failed to sign Checkpoint: %w", err)
	}
	if err := st.WriteCheckpoint(ctx, cpNoteSigned); err != nil {
		return fmt.Errorf("failed to store new log checkpoint: %w", err)
	}
	return nil
}
