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

// Package fs provides a simple filesystem log storage implementation.
package fs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/google/trillian-examples/serverless/api"
)

// FS is a serverless storage implementation which uses files to store tree state.
// The on-disk structure is:
//  <rootDir>/leaves/xx/xx/xx/xx
//  <rootDir>/leaves/pending/...
//  <rootDir>/tree/<level>/xx/xx
//  <rootDir>/state
type FS struct {
	rootDir string
	nextSeq uint64
	state   api.LogState
}

const (
	leavesPathFmt        = "%02x/%02x/%02x/%02x"
	leavesPendingPathFmt = "pending/%032x"
	subtreePathFtm       = "tree/%02x/%02x/%02x"
	statePath            = "state"
)

// New returns an FS instance initialised from the filesystem.
func New(rootDir string) (*FS, error) {
	fi, err := os.Stat(rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to stat %q: %w", rootDir, err)
	}

	if !fi.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", rootDir)
	}

	s, err := loadLogState(filepath.Join(rootDir, statePath))
	if err != nil {
		return nil, err
	}

	return &FS{
		rootDir: rootDir,
		state:   *s,
		nextSeq: s.Size,
	}, nil
}

// Create creates a new filesystem hierarchy and returns an FS representation for it.
func Create(rootDir string, emptyHash []byte) (*FS, error) {
	_, err := os.Stat(rootDir)
	if err == nil {
		return nil, fmt.Errorf("%q already exists", rootDir)
	}

	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory %q: %w", rootDir, err)
	}

	for _, sfx := range []string{"pending", "tree"} {
		path := filepath.Join(rootDir, sfx)
		if err := os.MkdirAll(path, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %q: %w", path, err)
		}
	}

	logState := api.LogState{
		Size:     0,
		RootHash: emptyHash,
		Hashes:   [][]byte{},
	}
	lsRaw, err := json.Marshal(logState)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal LogState: %w", err)
	}
	if err := ioutil.WriteFile(filepath.Join(rootDir, statePath), lsRaw, 0644); err != nil {
		return nil, fmt.Errorf("failed to write LogState: %w", err)
	}

	return &FS{
		rootDir: rootDir,
		state:   logState,
		nextSeq: logState.Size,
	}, nil
}

// LogState returns the current LogState.
func (fs *FS) LogState() api.LogState {
	return fs.state
}

// Sequence assigns the given leafhash and entry to the next available sequence number.
func (fs *FS) Sequence(leafHash []byte, leaf []byte) (uint64, error) {
	return 0, errors.New("unimplemented")
}

func loadLogState(s string) (*api.LogState, error) {
	raw, err := ioutil.ReadFile(s)
	if err != nil {
		return nil, err
	}

	var ls api.LogState
	if err := json.Unmarshal(raw, &ls); err != nil {
		return nil, fmt.Errorf("failed to unmarshal logstate: %w", err)
	}
	return &ls, nil
}
