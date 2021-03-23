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
	"github.com/google/trillian-examples/serverless/internal/storage"
	"github.com/google/trillian-examples/serverless/internal/storage/fs/layout"
)

// FS is a serverless storage implementation which uses files to store tree state.
// The on-disk structure is:
//  <rootDir>/leaves/aa/bb/cc/ddeeff...
//  <rootDir>/leaves/pending/
//  <rootDir>/seq/aa/bb/cc/ddeeff...
//  <rootDir>/tile/<level>/aa/bb/ccddee...
//  <rootDir>/state
type FS struct {
	rootDir string
	nextSeq uint64
	state   api.LogState
}

const leavesPendingPathFmt = "leaves/pending/%0x"

// New returns an FS instance initialised from the filesystem.
func New(rootDir string) (*FS, error) {
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

	fs := &FS{
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
func (fs *FS) LogState() api.LogState {
	return fs.state
}

// UpdateState updates the stored log state.
func (fs *FS) UpdateState(newState api.LogState) error {
	fs.state = newState
	fs.nextSeq = newState.Size
	lsRaw, err := json.Marshal(newState)
	if err != nil {
		return fmt.Errorf("failed to marshal LogState: %w", err)
	}
	return ioutil.WriteFile(filepath.Join(fs.rootDir, layout.StatePath), lsRaw, 0644)
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
	leafDir, leafFile := layout.LeafPath(fs.rootDir, leafhash)
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

		seqDir, seqFile := layout.SeqPath(fs.rootDir, seq)
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

// ScanSequenced calls the provided function once for each contiguous entry
// in storage starting at begin.
// The scan will abort if the function returns an error.
func (fs *FS) ScanSequenced(begin uint64, f func(seq uint64, entry []byte) error) (uint64, error) {
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

// GetTile returns the tile at the given tile-level and tile-index.
// If no complete tile exists at that location, it will attempt to find a
// partial tile for the given tree size at that location.
func (fs *FS) GetTile(level, index, logSize uint64) (*api.Tile, error) {
	tileSize := storage.PartialTileSize(level, index, logSize)
	p := filepath.Join(layout.TilePath(fs.rootDir, level, index, tileSize))
	t, err := ioutil.ReadFile(p)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("failed to read tile at %q: %w", p, err)
	}
	if err != nil {
		return nil, err
	}
	return parseTile(t)
}

func parseTile(t []byte) (*api.Tile, error) {
	var tile api.Tile
	if err := json.Unmarshal(t, &tile); err != nil {
		return nil, fmt.Errorf("failed to parse tile: %w", err)
	}
	return &tile, nil
}

// StoreTile writes a tile out to disk.
func (fs *FS) StoreTile(level, index uint64, tile *api.Tile) error {
	tileSize := storage.TileSize(tile)
	glog.V(2).Infof("StoreTile: level %d index %x ts: %x", level, index, tileSize)
	if tileSize == 0 || tileSize > 256 {
		return fmt.Errorf("tileSize %d must be > 0 and <= 256", tileSize)
	}
	t, err := json.Marshal(tile)
	if err != nil {
		return fmt.Errorf("failed to marshal tile: %w", err)
	}

	tDir, tFile := layout.TilePath(fs.rootDir, level, index, tileSize%256)
	tPath := filepath.Join(tDir, tFile)

	if err := os.MkdirAll(tDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", tDir, err)
	}

	// TODO(al): use unlinked temp file
	temp := fmt.Sprintf("%s.temp", tPath)
	if err := ioutil.WriteFile(temp, t, 0644); err != nil {
		return fmt.Errorf("failed to write temporary tile file: %w", err)
	}
	if err := os.Rename(temp, tPath); err != nil {
		return fmt.Errorf("failed to rename temporary tile file: %w", err)
	}

	// TODO(al): When tileSize == 256 attempt to clean up old partial tiles by making them be links to the full tile.

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
