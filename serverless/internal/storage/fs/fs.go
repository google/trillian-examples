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
//  <rootDir>/leaves/pending/
//  <rootDir>/seq/xx/xx/xx/xx
//  <rootDir>/tree/<level>/xx/xx
//  <rootDir>/state
type FS struct {
	rootDir string
	nextSeq uint64
	state   api.LogState
}

const (
	leavesPathFmt        = "leaves/%02x/%02x/%02x/%02x"
	leavesPendingPathFmt = "leaves/pending/%0x"
	seqPathFmt           = "seq/%02x/%02x/%02x/%02x"
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
		return nil, fmt.Errorf("%q %w", rootDir, os.ErrExist)
	}

	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory %q: %w", rootDir, err)
	}

	for _, sfx := range []string{"leaves/pending", "seq", "tree"} {
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

func seqPath(root string, seq uint64) (string, string) {
	frag := []string{
		root,
		"seq",
		fmt.Sprintf("%02x", (seq>>24)&0xff),
		fmt.Sprintf("%02x", (seq>>16)&0xff),
		fmt.Sprintf("%02x", (seq>>8)&0xff),
		fmt.Sprintf("%02x", seq&0xff),
	}
	d := filepath.Join(frag[:4]...)
	return d, frag[4]
}

func leafPath(root string, leafhash []byte) (string, string) {
	frag := []string{
		root,
		"leaves",
		fmt.Sprintf("%02x", leafhash[0]),
		fmt.Sprintf("%02x", leafhash[1]),
		fmt.Sprintf("%02x", leafhash[2]),
		fmt.Sprintf("%02x", leafhash[3]),
	}
	d := filepath.Join(frag[:4]...)
	return d, frag[4]
}

// Sequence assigns the given leaf entry to the next available sequence number.
func (fs *FS) Sequence(leafhash []byte, leaf []byte) error {
	// First store the entry in a temp file
	tmp := filepath.Join(fs.rootDir, fmt.Sprintf(leavesPendingPathFmt, leafhash))
	if err := ioutil.WriteFile(tmp, leaf, 0644); err != nil {
		return fmt.Errorf("unable to write leafdata to temporary file: %w", err)
	}
	defer func() {
		os.Remove(tmp)
	}()

	// Try to link into leaf data storage
	leafDir, leafFile := leafPath(fs.rootDir, leafhash)
	if err := os.MkdirAll(leafDir, 0755); err != nil {
		return fmt.Errorf("failed to make leaf directory structure: %w", err)
	}
	if err := os.Link(tmp, filepath.Join(leafDir, leafFile)); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("failed to link leafdata file: %w", err)
	}

	// Now try to sequence it, we may have to scan over some newly sequenced entries
	// if Sequence has been called since the last time an Integrate/UpdateState
	// was called.
	for {
		seq := fs.nextSeq

		seqDir, seqFile := seqPath(fs.rootDir, seq)
		if err := os.MkdirAll(seqDir, 0755); err != nil {
			return fmt.Errorf("failed to make seq directory structure: %w", err)
		}
		err := os.Link(tmp, filepath.Join(seqDir, seqFile))
		if errors.Is(err, os.ErrExist) {
			fs.nextSeq++
			continue
		} else if err != nil {
			return fmt.Errorf("failed to link seq file: %w", err)
		}
		break
	}

	return nil
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
