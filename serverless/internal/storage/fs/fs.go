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
	"github.com/google/trillian-examples/serverless/internal/storage/fs/layout"
)

const (
	dirPerm  = 0755
	filePerm = 0644
)

// Storage is a serverless storage implementation which uses files to store tree state.
// The on-disk structure is:
//  <rootDir>/leaves/aa/bb/cc/ddeeff...
//  <rootDir>/seq/aa/bb/cc/ddeeff...
//  <rootDir>/tile/<level>/aa/bb/ccddee...
//  <rootDir>/state
//
// The functions on this struct are not thread-safe.
type Storage struct {
	rootDir string
	state   api.LogState
}

// Load returns an Storage instance initialised from the filesystem.
func Load(rootDir string) (*Storage, error) {
	fi, err := os.Stat(rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to stat %q: %w", rootDir, err)
	}

	if !fi.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", rootDir)
	}

	s, err := loadLogState(filepath.Join(rootDir, layout.StatePath))
	if err != nil {
		return nil, err
	}

	return &Storage{
		rootDir: rootDir,
		state:   *s,
	}, nil
}

// Create creates a new filesystem hierarchy and returns a Storage representation for it.
func Create(rootDir string, emptyHash []byte) (*Storage, error) {
	_, err := os.Stat(rootDir)
	if err == nil {
		return nil, fmt.Errorf("%q %w", rootDir, os.ErrExist)
	}

	if err := os.MkdirAll(rootDir, dirPerm); err != nil {
		return nil, fmt.Errorf("failed to create directory %q: %w", rootDir, err)
	}

	fs := &Storage{
		rootDir: rootDir,
	}

	logState := api.LogState{
		Size:     0,
		RootHash: emptyHash,
		Hashes:   [][]byte{},
	}

	if err := fs.UpdateState(logState); err != nil {
		return nil, err
	}
	return fs, nil
}

// LogState returns the current LogState.
func (fs *Storage) LogState() api.LogState {
	return fs.state
}

// UpdateState updates the stored log state.
func (fs *Storage) UpdateState(newState api.LogState) error {
	fs.state = newState
	lsRaw, err := json.Marshal(newState)
	if err != nil {
		return fmt.Errorf("failed to marshal LogState: %w", err)
	}
	oPath := filepath.Join(fs.rootDir, layout.StatePath)
	tmp := fmt.Sprintf("%s.tmp", oPath)
	if err := ioutil.WriteFile(tmp, lsRaw, filePerm); err != nil {
		return fmt.Errorf("failed to create temporary state file: %w", err)
	}
	return os.Rename(tmp, oPath)
}

// Sequence assigns the given leaf entry to the next available sequence number.
func (fs *Storage) Sequence(leafhash []byte, leaf []byte) error {
	return errors.New("unimplemented")
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
