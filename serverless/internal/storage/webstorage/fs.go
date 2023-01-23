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

//go:build wasm
// +build wasm

// Package webstorage provides a simple log storage implementation based on webstorage.
// It only really makes sense for wasm targets in browsers where the sessionStorage
// WebStorage API is available.
// See https://developer.mozilla.org/en-US/docs/Web/API/Web_Storage_API for more information
// about the browser WebStorage API.
package webstorage

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/google/trillian-examples/serverless/api"
	"github.com/google/trillian-examples/serverless/api/layout"
	"github.com/google/trillian-examples/serverless/pkg/log"

	fmtlog "github.com/transparency-dev/formats/log"
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
	checkpoint fmtlog.Checkpoint
}

const leavesPendingPathFmt = "leaves/pending/%0x"

func getStorage() js.Value {
	return js.Global().Get("sessionStorage")
}

func exists(k string) bool {
	_, err := get(k)
	return err == nil
}

func get(k string) ([]byte, error) {
	v := getStorage().Call("getItem", js.ValueOf(k))
	if js.Null().Equal(v) {
		return nil, os.ErrNotExist
	}
	return []byte(v.String()), nil
}

func set(k string, v []byte) error {
	getStorage().Call("setItem", js.ValueOf(k), js.ValueOf(string(v)))
	// TODO(al) read back and check
	return nil
}

func createExclusive(k string, v []byte) error {
	if exists(k) {
		return os.ErrExist
	}
	getStorage().Call("setItem", js.ValueOf(k), js.ValueOf(string(v)))
	// TODO(al) read back and check
	return nil
}

// Load returns a Storage instance initialised from webstorage prefixed at root.
// cpSize should be the Size of the checkpoint produced from the last `log.Integrate` call.
func Load(root string, cpSize uint64) (*Storage, error) {
	return &Storage{
		root:    root,
		nextSeq: cpSize,
	}, nil
}

// Create creates a new filesystem hierarchy and returns a Storage representation for it.
func Create(root string) (*Storage, error) {
	fs := &Storage{
		root:    root,
		nextSeq: 0,
	}

	return fs, nil
}

// Queue adds a leaf to the pending queue for integration.
func (fs *Storage) Queue(leaf []byte) error {
	h := sha256.Sum256(leaf)
	return createExclusive(filepath.Join(fs.root, fmt.Sprintf(leavesPendingPathFmt, h)), leaf)
}

// PendingKeys returns the storage keys associated with pending leaves.
func (fs *Storage) PendingKeys() ([]string, error) {
	pk := []string{}
	s := getStorage()
	prefix := filepath.Join(fs.root, "leaves", "pending")
	for i := 0; i < s.Get("length").Int(); i++ {
		k := s.Call("key", js.ValueOf(i))
		if js.Null().Equal(k) {
			return nil, fmt.Errorf("key(%d) failed", i)
		}
		ks := k.String()
		if strings.HasPrefix(ks, prefix) {
			pk = append(pk, ks)
		}
	}
	return pk, nil
}

// Pending returns a pending leaf stored under PendingKey.
func (fs *Storage) Pending(f string) ([]byte, error) {
	prefix := filepath.Join(fs.root, "leaves", "pending")
	if !strings.HasPrefix(f, prefix) {
		return nil, fmt.Errorf("pending key %q does not have prefix %q", f, prefix)
	}
	return get(f)
}

// DeletePending removes a pending leaf stored under PendingKey.
func (fs *Storage) DeletePending(f string) error {
	prefix := filepath.Join(fs.root, "leaves", "pending")
	if !strings.HasPrefix(f, prefix) {
		return fmt.Errorf("pending key %q does not have prefix %q", f, prefix)
	}
	getStorage().Call("removeItem", f)
	return nil
}

// Sequence assigns the given leaf entry to the next available sequence number.
// This method will attempt to silently squash duplicate leaves, but it cannot
// be guaranteed that no duplicate entries will exist.
// Returns the sequence number assigned to this leaf (if the leaf has already
// been sequenced it will return the original sequence number and ErrDupeLeaf).
func (fs *Storage) Sequence(_ context.Context, leafhash []byte, leaf []byte) (uint64, error) {
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
		return origSeq, log.ErrDupeLeaf
	}

	// Now try to sequence it, we may have to scan over some newly sequenced entries
	// if Sequence has been called since the last time an Integrate/WriteCheckpoint
	// was called.
	for {
		seq := fs.nextSeq

		// Ensure the sequencing directory structure is present:
		seqDir, seqFile := layout.SeqPath(fs.root, seq)

		// Write the newly sequenced leaf
		seqPath := filepath.Join(seqDir, seqFile)
		err := createExclusive(seqPath, leaf)
		if err != nil {
			if !errors.Is(err, os.ErrExist) {
				return 0, fmt.Errorf("failed to store sequenced leaf: %v", err)
			}
			fs.nextSeq++
			continue
		}

		// Create a leafhash file containing the assigned sequence number.
		// This isn't infallible though, if we crash after hardlinking the
		// sequence file above, but before doing this a resubmission of the
		// same leafhash would be permitted.
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
func (fs *Storage) ScanSequenced(_ context.Context, begin uint64, f func(seq uint64, entry []byte) error) (uint64, error) {
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
func (fs *Storage) GetTile(_ context.Context, level, index, logSize uint64) (*api.Tile, error) {
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
func (fs *Storage) StoreTile(_ context.Context, level, index uint64, tile *api.Tile) error {
	tileSize := uint64(tile.NumLeaves)
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
func (fs Storage) WriteCheckpoint(_ context.Context, newCPRaw []byte) error {
	oPath := filepath.Join(fs.root, layout.CheckpointPath)
	return set(oPath, newCPRaw)
}

// ReadCheckpoint reads and returns the contents of the log checkpoint file.
func ReadCheckpoint(root string) ([]byte, error) {
	s := filepath.Join(root, layout.CheckpointPath)
	return get(s)
}
