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

// merge_witness_signatures is a tool to manage the checkpoint.witnessed file.
//
// Given the following files:
// - checkpoint: a log checkpoint
// - checkpoint.witnessed: a log checkpoint with additional witness signatures (optional)
// - a collection of additional witnessed checkpoints
// this tool will determine whether to update an existing checkpoint.witnessed
// from the additional witnessed checkpoints, or replace it entirely by merging
// witness cosignatures with the contents of the checkepoint file.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/formats/checkpoints"
	"golang.org/x/mod/sumdb/note"
)

var (
	storageDir     = flag.String("storage_dir", "", "Root directory of the log.")
	logKey         = flag.String("log_public_key", "", "Log's public key.")
	witnessKeys    = flag.String("witness_public_key_files", "", "One or more space-separated globs matching all the known witness keys.")
	numRequired    = flag.Uint("required_witness_sigs", 0, "Specifies the minimum number of valid witness signatures required to be present on the output to consider the merge successful.")
	output         = flag.String("output", "", "Output file to write combined checkpoint to.")
	deleteConsumed = flag.Bool("delete_consumed", true, "Whether to delete files containing merged checkpoints.")
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

	cpNote, cpRaw, err := openNote(filepath.Join(*storageDir, "checkpoint"), note.VerifierList(logSigV))
	if err != nil {
		glog.Exitf("Failed to open checkpoint: %v", err)
	}

	cpwNote, cpwRaw, err := openNote(filepath.Join(*storageDir, "checkpoint.witnessed"), witSigV)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			glog.Exitf("Failed to open checkpoint.witnessed: %v", err)
		}
	}

	witnessDir := filepath.Join(*storageDir, "witness")

	ans, err := additionalNotes(filepath.Join(witnessDir, "*"))
	if err != nil {
		glog.Exitf("Failed to read additional checkpoints: %v", err)
	}

	var out []byte
	// If checkpoints are different (or there is no checkpoint.witnessed file), see
	// if we have sufficient witness cosigs to promote checkpoint to
	// checkpoint.witnessed
	if cpwNote == nil || cpNote.Text != cpwNote.Text {
		out, err = mergeCheckpoints(cpRaw, ans, *numRequired, logSigV, witSigV, false)
		if err != nil {
			glog.Warningf("Failed to merge with new checkpoint file: %v", err)
		} else {
			// Since we've just promoted the new checkpoint and merged any compatible
			// cosignatures, we can delete all additional Checkpoints:
			if err := os.RemoveAll(witnessDir); err != nil {
				glog.Warningf("Failed to remove witness directory: %v", err)
			}
		}
	}

	// If we failed above, or the checkpoint and checkpoint.witnessed files are
	// equivalent, try to merge more sigs into checkpoint.witnessed
	if len(out) == 0 && cpwNote != nil {
		out, err = mergeCheckpoints(cpwRaw, ans, 0, logSigV, witSigV, *deleteConsumed)
		if err != nil {
			glog.Warningf("Failed to merge with checkpoint.witnessed file: %v", err)
		}
		// Note that we _don't_ delete everything from the `witness/` directory here since
		// there could be some cosigs over `checkpoint` but not enough for the merge & promote
		// step above - we want to keep those around until we get more.
	}

	if len(out) == 0 {
		glog.Infof("Not updating %q", *output)
		return
	}

	if err := ioutil.WriteFile(*output, out, 644); err != nil {
		glog.Exitf("Failed to write output checkpoint to %q: %v", *output, err)
	}
}

func mergeCheckpoints(into []byte, additional map[string][]byte, n uint, logSigV note.Verifier, wSigV note.Verifiers, deleteConsumed bool) ([]byte, error) {
	cp, err := note.Open(into, note.VerifierList(logSigV))
	if err != nil {
		return nil, err
	}
	good := make([][]byte, 0)
	toDelete := make([]string, 0)
	for wName, wRaw := range additional {
		if strings.HasPrefix(string(wRaw), cp.Text) {
			good = append(good, wRaw)
			toDelete = append(toDelete, wName)
		}
	}
	outRaw, err := checkpoints.Combine(append([][]byte{into}, good...), logSigV, wSigV)
	if err != nil {
		glog.Exitf("Failed to combine checkpoints: %v", err)
	}
	out, err := note.Open(outRaw, wSigV)
	if err != nil {
		return nil, fmt.Errorf("failed to open combined checkpoint: %v", err)
	}
	if got, want := len(out.Sigs), int(n); got < want {
		return nil, fmt.Errorf("insufficient signatures, got %d < want %d", got, want)
	}
	if deleteConsumed {
		for _, n := range toDelete {
			if err := os.Remove(n); err != nil {
				glog.Warningf("Failed to delete consumed witness checkpoint %q: %v", n, err)
			}
		}
	}
	return outRaw, nil

}

func openNote(f string, v note.Verifiers) (*note.Note, []byte, error) {
	r, err := ioutil.ReadFile(f)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read note %q: %v", f, err)
	}
	n, err := note.Open(r, v)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open note %q: %v", f, err)
	}
	return n, r, nil
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

func additionalNotes(globs string) (map[string][]byte, error) {
	ret := make(map[string][]byte, 0)
	err := readGlobs(globs, func(f string, b []byte) error {
		ret[f] = b
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
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
