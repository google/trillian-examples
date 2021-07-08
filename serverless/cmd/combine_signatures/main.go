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

// combine_signatures is a tool to combine signatures of two or more equivalent signed checkpoints.
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/serverless/cmd/combine_signatures/impl"
	"golang.org/x/mod/sumdb/note"
)

var (
	logKey      = flag.String("log_public_key", "", "Log's public key")
	witnessKeys = flag.String("witness_public_key_filess", "", "One or more space-separated globs matching all the known witness keys.")
	output      = flag.String("output", "", "Output file to write combined checkpoint to.")
)

func main() {
	flag.Parse()

	if len(flag.Args()) < 2 {
		glog.Exitf("Usage: combine_signatures <checkpoint_file_1> <checkpoint_file_2> [checkpoint_file_N ...")
	}

	logSigV, err := note.NewVerifier(*logKey)
	if err != nil {
		glog.Exitf("Failed to parse log public key: %v", err)
	}

	witSigV, err := witnessVerifiers(*witnessKeys)
	if err != nil {
		glog.Exitf("Failed to load witness keys: %v", err)
	}

	cps := make([][]byte, 0, len(flag.Args()))

	for _, f := range flag.Args() {
		r, err := ioutil.ReadFile(f)
		if err != nil {
			glog.Exitf("%v: %v", f, err)
		}
		cps = append(cps, r)
	}

	o, err := impl.Combine(cps, logSigV, witSigV)
	if err != nil {
		glog.Exitf("Failed to combine checkpoints: %v", err)
	}

	if err := ioutil.WriteFile(*output, o, 644); err != nil {
		glog.Exitf("Failed to write output checkpoint to %q: %v", *output, err)
	}
}

func witnessVerifiers(globs string) (note.Verifiers, error) {
	ret := make([]note.Verifier, 0)
	g := strings.Split(globs, " ")
	for _, glob := range g {
		files, err := filepath.Glob(glob)
		if err != nil {
			return nil, fmt.Errorf("glob %q failed: %v", glob, err)
		}
		for _, f := range files {
			raw, err := ioutil.ReadFile(f)
			if err != nil {
				return nil, fmt.Errorf("failed to read witness key file %q: %v", f, err)
			}
			v, err := note.NewVerifier(string(raw))
			if err != nil {
				return nil, fmt.Errorf("invalid witness key file %q: %v", f, err)
			}
			ret = append(ret, v)
		}
	}
	return note.VerifierList(ret...), nil
}
