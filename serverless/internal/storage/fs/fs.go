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

	"github.com/golang/glog"
	"github.com/google/trillian-examples/serverless/api"
	"github.com/google/trillian-examples/serverless/internal/storage/fs/layout"
)

const (
	dirPerm  = 0755
	filePerm = 0644
	// TODO(al): consider making immutable files completely readonly
)

// Storage is a serverless storage implementation which uses files to store tree state.
// The on-disk structure is:
//  <rootDir>/leaves/aa/bb/cc/ddeeff...
//  <rootDir>/leaves/pending/aabbccddeeff...
//  <rootDir>/seq/aa/bb/cc/ddeeff...
//  <rootDir>/tile/<level>/aa/bb/ccddee...
//  <rootDir>/state
//
// The functions on this struct are not thread-safe.
type Storage struct {
	rootDir string
	nextSeq uint64
	state   api.LogState
}

const leavesPendingPathFmt = "leaves/pending/%0x"

// Load returns a Storage instance initialised from the filesystem.
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
		nextSeq: s.Size,
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

	for _, sfx := range []string{"leaves/pending", "seq", "tile"} {
		path := filepath.Join(rootDir, sfx)
		if err := os.MkdirAll(path, dirPerm); err != nil {
			return nil, fmt.Errorf("failed to create directory %q: %w", path, err)
		}
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
	fs.nextSeq = newState.Size
	lsRaw, err := json.Marshal(newState)
	if err != nil {
		return fmt.Errorf("failed to marshal LogState: %w", err)
	}
	oPath := filepath.Join(fs.rootDir, layout.StatePath)
	tmp := fmt.Sprintf("%s.tmp", oPath)
	if err := createExcl(tmp, lsRaw); err != nil {
		return fmt.Errorf("failed to create temporary state file: %w", err)
	}
	return os.Rename(tmp, oPath)
}

// Sequence assigns the given leaf entry to the next available sequence number.
// This method will attempt to silently squash duplicate leaves, but it cannot
// be guaranteed that no duplicate entries will exist.
func (fs *Storage) Sequence(leafhash []byte, leaf []byte) error {
	// 1. Check for dupe leafhash
	// 2. Write temp file
	// 3. Hard link temp -> seq file
	// 4. Optimistically symlink leafhash -> seq file

	// Ensure the leafhash diretory structure is pressent
	leafDir, leafFile := layout.LeafPath(fs.rootDir, leafhash)
	if err := os.MkdirAll(leafDir, dirPerm); err != nil {
		return fmt.Errorf("failed to make leaf directory structure: %w", err)
	}
	// Check for dupe leaf already present.
	leafFQ := filepath.Join(leafDir, leafFile)
	if _, err := os.Stat(leafFQ); os.IsExist(err) {
		glog.Infof("Skipping dupe leafhash 0x%x", leafhash)
		return nil
	}

	tmp := filepath.Join(fs.rootDir, fmt.Sprintf(leavesPendingPathFmt, leafhash))
	if err := createExcl(tmp, leaf); err != nil {
		return fmt.Errorf("unable to write temporary file: %w", err)
	}
	defer func() {
		os.Remove(tmp)
	}()

	// Now try to sequence it, we may have to scan over some newly sequenced entries
	// if Sequence has been called since the last time an Integrate/UpdateState
	// was called.
	for {
		seq := fs.nextSeq

		// Ensure the sequencing directory structure is present:
		seqDir, seqFile := layout.SeqPath(fs.rootDir, seq)
		if err := os.MkdirAll(seqDir, dirPerm); err != nil {
			return fmt.Errorf("failed to make seq directory structure: %w", err)
		}

		// Hardlink the sequence file to the temporary file
		seqPath := filepath.Join(seqDir, seqFile)
		if err := os.Link(tmp, seqPath); errors.Is(err, os.ErrExist) {
			// That sequence number is in use, try the next one
			fs.nextSeq++
			continue
		} else if err != nil {
			return fmt.Errorf("failed to link seq file: %w", err)
		}

		// Finally, try to link leafhash -> seq.
		err := os.Symlink(seqPath, leafFQ)
		if err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("failed to link leafdata file: %w", err)
		}

		// All done!
		break
	}

	return nil
}

// createExcl, creates the named file before writing the data in d to it.
// It will error if the file already exists, or it's unable to fully write the
// data & close the file.
func createExcl(f string, d []byte) error {
	tmpFile, err := os.OpenFile(f, os.O_RDWR|os.O_CREATE|os.O_EXCL, filePerm)
	if err != nil {
		return fmt.Errorf("unable to create temporary file: %w", err)
	}
	n, err := tmpFile.Write(d)
	if err != nil {
		return fmt.Errorf("unable to write leafdata to temporary file: %w", err)
	}
	if got, want := n, len(d); got != want {
		return fmt.Errorf("short write on leaf, wrote %d expected %d", got, want)
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	return nil
}

// ScanSequenced calls the provided function once for each contiguous entry
// in storage starting at begin.
// The scan will abort if the function returns an error.
func (fs *Storage) ScanSequenced(begin uint64, f func(seq uint64, entry []byte) error) (uint64, error) {
	end := begin
	for {
		sp := filepath.Join(layout.SeqPath(fs.rootDir, end))
		entry, err := ioutil.ReadFile(sp)
		if errors.Is(err, os.ErrNotExist) {
			// we're done.
			return end - begin, nil
		} else if err != nil {
			return end - begin, fmt.Errorf("failed to read leafdata at index %d: %w", begin, err)
		}
		if err := f(end, entry); err != nil {
			return end - begin, err
		}
		end++
	}
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
