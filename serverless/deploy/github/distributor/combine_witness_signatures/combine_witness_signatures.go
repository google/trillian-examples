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

// combine_witness_signatures is a tool to manage the distributor state files.
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/serverless/deploy/github/distributor/combine_witness_signatures/internal/distributor"
	"golang.org/x/mod/sumdb/note"
	"gopkg.in/yaml.v2"
)

var (
	storageDir = flag.String("distributor_dir", "", "Root directory of the distributor.")
	configFile = flag.String("config", "", "Distributor config file.")
	dryRun     = flag.Bool("dry_run", false, "Don't write or update any files, just validate.")
)

func main() {
	flag.Parse()

	opts, err := loadConfig(*configFile)
	if err != nil {
		glog.Exitf("Unable to load config from %q: %v", *configFile, err)
	}

	matcher, err := regexp.Compile(filepath.Join(*storageDir, "logs", "([^/]+)"))
	if err != nil {
		glog.Exitf("Failed to create matcher: %v", err)
	}
	incoming, err := filepath.Glob(filepath.Join(*storageDir, "logs", "*" /*log_id*/, "incoming"))
	if err != nil {
		glog.Exitf("Failed to match incoming checkpoint files: %v", err)
	}

	for _, i := range incoming {
		m := matcher.FindStringSubmatch(i)
		// First entry in m is the whole matched string, the second is the log ID
		if got, want := len(m), 2; got != want {
			glog.Exitf("Unexpected match on %q, got %d parts but want %d", i, got, want)
		}
		logID := m[1]

		o, ok := opts[logID]
		if !ok {
			glog.Exitf("Found incoming directory for unknown log %q", logID)
		}

		// Read state
		state, incoming, err := readState(m[0])
		if err != nil {
			glog.Exitf("Error reading distributor state for log %q: %v", logID, err)
		}
		newState, err := distributor.UpdateState(state, incoming, o)
		if err != nil {
			glog.Exitf("Error updating distributor state: %v", err)
		}
		if *dryRun {
			glog.Infof("Not writing new state to disk because --dry_run is set")
			return
		}
		if err := storeState(m[0], newState); err != nil {
			glog.Exitf("Error storing distributor state for log %q: %v", logID, err)
		}
	}
}

// readState loads the distributor state stored in the specified root directory.
// This state consists of the checkpoint.N files, along with any additional files
// stored under an incoming/ directory.
func readState(root string) ([][]byte, [][]byte, error) {
	sFiles, err := filepath.Glob(filepath.Join(root, "checkpoint.[0-9]"))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to glob checkpoints: %v", err)
	}
	iFiles, err := filepath.Glob(filepath.Join(root, "incoming", "*"))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to glob incoming checkpoints: %v", err)
	}

	state, err := readFiles(sFiles)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read state files: %v", err)
	}
	incoming, err := readFiles(iFiles)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read incoming files: %v", err)
	}
	return state, incoming, nil
}

// readFiles reads and returns the provided list of files.
func readFiles(f []string) ([][]byte, error) {
	r := make([][]byte, 0, len(f))
	for _, p := range f {
		b, err := ioutil.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("failed to read %q: %v", p, err)
		}
		r = append(r, b)
	}
	return r, nil
}

// storeState updates the distributor state stored in the specified directory.
// The state param must contain the ordered 0..N checkpoints which will replace
// any existing checkpoint.N files.
// The contents of the incoming/ directory will be removed.
func storeState(root string, state [][]byte) error {
	i := 0
	for ; i < len(state); i++ {
		o := filepath.Join(root, fmt.Sprintf("checkpoint.%d", i))
		if len(state[i]) > 0 {
			if err := os.WriteFile(o, state[i], 0o644); err != nil {
				return fmt.Errorf("error writing state[%d] in distributor directory %q: %v", i, root, err)
			}
		} else {
			_ = os.Remove(o)
		}
	}
	// clean up any unwanted old state files
	for ; i < 10; i++ {
		o := filepath.Join(root, fmt.Sprintf("checkpoint.%d", i))
		_ = os.Remove(o)
	}
	// clean up any unwanted old incoming files
	o := filepath.Join(root, "incoming")
	if err := os.RemoveAll(o); err != nil {
		return fmt.Errorf("failed to remove contents of %q: %v", o, err)
	}
	return nil
}

func loadConfig(f string) (map[string]distributor.UpdateOpts, error) {
	cfg := &struct {
		Logs []struct {
			ID        string `yaml:"ID"`
			PublicKey string `yaml:"PublicKey"`
			Origin    string `yaml:"Origin"`
		} `yaml:"Logs"`
		Witnesses            []string `yaml:"Witnesses"`
		MaxWitnessSignatures uint     `yaml:"MaxWitnessSignatures"`
	}{}
	raw, err := ioutil.ReadFile(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %q: %v", f, err)
	}
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}

	// Witnesses are currently valid for all logs in the config.
	witnesses := []note.Verifier{}
	for wi, w := range cfg.Witnesses {
		sv, err := note.NewVerifier(w)
		if err != nil {
			return nil, fmt.Errorf("failed to instantiate public key for witness at index %d: %v", wi, err)
		}
		witnesses = append(witnesses, sv)
	}

	ret := make(map[string]distributor.UpdateOpts)
	for li, l := range cfg.Logs {
		sv, err := note.NewVerifier(l.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("failed to instantiate public key for log at index %d: %v", li, err)
		}
		ret[l.ID] = distributor.UpdateOpts{
			MaxWitnessSignatures: cfg.MaxWitnessSignatures,
			LogSigV:              sv,
			LogOrigin:            l.Origin,
			Witnesses:            witnesses,
		}
	}
	return ret, nil
}
