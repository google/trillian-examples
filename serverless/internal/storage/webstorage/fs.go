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

// Package webstorage provides a simple log storage implementation based on webstorage.
// It only really makes sense for wasm targets.
// +build wasm
package fs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall/js"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/serverless/api"
	"github.com/google/trillian-examples/serverless/api/layout"
	"github.com/google/trillian-examples/serverless/internal/storage"
)

// Storage is a serverless storage implementation which uses webstorage entries to store tree state.
// The storage key format is:
//  <root>/leaves/aa/bb/cc/ddeeff...
//  <root>/leaves/pending/aabbccddeeff...
//  <root>/seq/aa/bb/cc/ddeeff...
//  <root>/tile/<level>/aa/bb/ccddee...
//  <root>/checkpoint
//
// The functions on this struct are not thread-safe.
type Storage struct {
	// root is the prefix of all webstorage keys which store tree data.
	root string
	// nextSeq is a hint to the Sequence func as to what the next available
	// sequence number is to help performance.
	// Note that nextSeq may be <= than the actual next available number, but
	// never greater.
	nextSeq uint64
	// checkpoint is the latest known checkpoint of the log.
	checkpoint log.Checkpoint
}

const leavesPendingPathFmt = "leaves/pending/%0x"

func exists(k string) bool {
	_, err := get(k)
	return err == nil
}

func get(k string) ([]byte, error) {
	v := js.Global().Get("sessionStorage").Call("getItem", k)
	if v.Undefined() {
		return nil, os.ErrNotExist
	}
	return []byte(v.String()), nil
}

func set(k string, v []byte) error {
	js.Global().Get("sessionStorage").Call("setItem", []string{k, string(v)})
	// TODO(al) read back and check
	return nil
}

func createExclusive(k string, v []byte) error {
	if exists(k) {
		return os.ErrExist
	}
	js.Global().Get("sessionStorage").Call("setItem", []string{k, string(v)})
	// TODO(al) read back and check
	return nil
}

// Load returns a Storage instance initialised from webstorage prefixed at root.
func Load(root string, checkpoint *log.Checkpoint) (*Storage, error) {
	return &Storage{
		root:       root,
		checkpoint: *checkpoint,
		nextSeq:    checkpoint.Size,
	}, nil
}

// Create creates a new filesystem hierarchy and returns a Storage representation for it.
func Create(root string, emptyHash []byte) (*Storage, error) {
	fs := &Storage{
		root:    root,
		nextSeq: 0,
		checkpoint: log.Checkpoint{
			Size: 0,
			Hash: emptyHash,
		},
	}

	return fs, nil
}

// Checkpoint returns the current Checkpoint.
func (fs *Storage) Checkpoint() log.Checkpoint {
	return fs.checkpoint
}

// Sequence assigns the given leaf entry to the next available sequence number.
// This method will attempt to silently squash duplicate leaves, but it cannot
// be guaranteed that no duplicate entries will exist.
// Returns the sequence number assigned to this leaf (if the leaf has already
// been sequenced it will return the original sequence number and
// storage.ErrDupeLeaf).
func (fs *Storage) Sequence(leafhash []byte, leaf []byte) (uint64, error) {
	// 1. Check for dupe leafhash
	// 2. Write temp file
	// 3. Hard link temp -> seq file
	// 4. Create leafhash file containing assigned sequence number

	leafDir, leafFile := layout.LeafPath(fs.root, leafhash)
	// Check for dupe leaf already present.
	// If there is one, it should contain the existing leaf's sequence number,
	// so read that back and return it.
	leafFQ := filepath.Join(leafDir, leafFile)
	if seqString, err := get(leafFQ); !os.IsNotExist(err) {
		origSeq, err := strconv.ParseUint(string(seqString), 16, 64)
		if err != nil {
			return 0, err
		}
		return origSeq, storage.ErrDupeLeaf
	}

	// Now try to sequence it, we may have to scan over some newly sequenced entries
	// if Sequence has been called since the last time an Integrate/WriteCheckpoint
	// was called.
	for {
		seq := fs.nextSeq

		// Ensure the sequencing directory structure is present:
		seqDir, seqFile := layout.SeqPath(fs.root, seq)

		// Hardlink the sequence file to the temporary file
		seqPath := filepath.Join(seqDir, seqFile)
		if err := createExclusive(seqPath, leaf); err != nil {
			return 0, fmt.Errorf("failed to store sequenced leaf: %v", err)
		}

		// Create a leafhash file containing the assigned sequence number.
		// This isn't infallible though, if we crash after hardlinking the
		// sequence file above, but before doing this a resubmission of the
		// same leafhash would be permitted.
		//
		if err := createExclusive(leafFQ, []byte(strconv.FormatUint(seq, 16))); err != nil {
			return 0, fmt.Errorf("couldn't create temporary leafhash file: %w", err)
		}
		// All done!
		return seq, nil
	}
}

// ScanSequenced calls the provided function once for each contiguous entry
// in storage starting at begin.
// The scan will abort if the function returns an error, otherwise it will
// return the number of sequenced entries.
func (fs *Storage) ScanSequenced(begin uint64, f func(seq uint64, entry []byte) error) (uint64, error) {
	end := begin
	for {
		sp := filepath.Join(layout.SeqPath(fs.root, end))
		entry, err := get(sp)
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
func (fs *Storage) GetTile(level, index, logSize uint64) (*api.Tile, error) {
	tileSize := layout.PartialTileSize(level, index, logSize)
	p := filepath.Join(layout.TilePath(fs.root, level, index, tileSize))
	t, err := get(p)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("failed to read tile at %q: %w", p, err)
		}
		return nil, err
	}

	var tile api.Tile
	if err := tile.UnmarshalText(t); err != nil {
		return nil, fmt.Errorf("failed to parse tile: %w", err)
	}
	return &tile, nil
}

// StoreTile writes a tile out to disk.
// Fully populated tiles are stored at the path corresponding to the level &
// index parameters, partially populated (i.e. right-hand edge) tiles are
// stored with a .xx suffix where xx is the number of "tile leaves" in hex.
func (fs *Storage) StoreTile(level, index uint64, tile *api.Tile) error {
	tileSize := uint64(tile.NumLeaves)
	glog.V(2).Infof("StoreTile: level %d index %x ts: %x", level, index, tileSize)
	if tileSize == 0 || tileSize > 256 {
		return fmt.Errorf("tileSize %d must be > 0 and <= 256", tileSize)
	}
	t, err := tile.MarshalText()
	if err != nil {
		return fmt.Errorf("failed to marshal tile: %w", err)
	}

	tDir, tFile := layout.TilePath(fs.root, level, index, tileSize%256)
	tPath := filepath.Join(tDir, tFile)

	if err := createExclusive(tPath, t); err != nil {
		return fmt.Errorf("failed to write temporary tile file: %w", err)
	}

	return nil
}

// WriteCheckpoint stores a raw log checkpoint on disk.
func (fs Storage) WriteCheckpoint(newCPRaw []byte) error {
	oPath := filepath.Join(fs.root, layout.CheckpointPath)
	return createExclusive(oPath, newCPRaw)
}

// ReadCheckpoint reads and returns the contents of the log checkpoint file.
func ReadCheckpoint(root string) ([]byte, error) {
	s := filepath.Join(root, layout.CheckpointPath)
	return get(s)
}
