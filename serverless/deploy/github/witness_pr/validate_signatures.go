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

// validate_signatures is a tool check signatures on a note.
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
	"golang.org/x/mod/sumdb/note"
)

var (
	checkpointPath = flag.String("checkpoint", "", "Path to checkpoint file to verify.")
	logKey         = flag.String("log_public_key", "", "Log's public key.")
	witnessKeys    = flag.String("witness_public_key_files", "", "One or more space-separated globs matching all the known witness keys.")
)

func main() {
	flag.Parse()

	logSigV, err := note.NewVerifier(*logKey)
	if err != nil {
		glog.Exitf("Failed to parse log public key: %v", err)
	}

	witSigV, err := witnessVerifiers(*witnessKeys)
	if err != nil {
		glog.Exitf("Failed to load witness keys: %v", err)
	}

	n, err := openNote(*checkpointPath, logSigV, witSigV)
	if err != nil {
		glog.Exitf("Invalid checkpoint: %v", err)
	}
	glog.Infof("::debug:Found checkpoint signed by %q", n.Sigs[0].Name)
}

func openNote(f string, logSigV note.Verifier, wSigV note.Verifiers) (*note.Note, error) {
	r, err := ioutil.ReadFile(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read note %q: %v", f, err)
	}
	_, err = note.Open(r, note.VerifierList(logSigV))
	if err != nil {
		return nil, fmt.Errorf("failed to open note with log key: %v", f, err)
	}
	n, err := note.Open(r, wSigV)
	if err != nil {
		return nil, fmt.Errorf("failed to open note with witness keys: %v", f, err)
	}
	return n, nil
}

func witnessVerifiers(globs string) (note.Verifiers, error) {
	vs := make([]note.Verifier, 0)
	err := readGlobs(globs, func(f string, b []byte) error {
		v, err := note.NewVerifier(string(b))
		if err != nil {
			return fmt.Errorf("invalid witness key file %q: %v", f, err)
		}
		vs = append(vs, v)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return note.VerifierList(vs...), nil
}

func readGlobs(globs string, f func(n string, b []byte) error) error {
	for _, glob := range strings.Split(globs, " ") {
		files, err := filepath.Glob(glob)
		if err != nil {
			return fmt.Errorf("glob %q failed: %v", glob, err)
		}
		for _, fn := range files {
			raw, err := ioutil.ReadFile(fn)
			if err != nil {
				return fmt.Errorf("failed to read witness key file %q: %v", fn, err)
			}
			if err := f(fn, raw); err != nil {
				return fmt.Errorf("f() = %v", err)
			}
		}
	}
	return nil
}
