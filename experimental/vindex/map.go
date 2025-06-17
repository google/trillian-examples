// Copyright 2025 Google LLC. All Rights Reserved.
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

// vindex contains a prototype of an in-memory verifiable index.
// It reads data from an InputLog interface, applies a MapFn to every leaf in the
// input log, and writes the mapped information out to a Write Ahead Log. Data is
// read from the WAL, and the in-memory map is built from this.
package vindex

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"filippo.io/torchwood/mpt"
	"github.com/transparency-dev/formats/log"
	"golang.org/x/sync/errgroup"
	"k8s.io/klog/v2"
)

// MapFn takes the raw leaf data from a log entry and outputs the SHA256 hashes
// of the keys at which this leaf should be indexed under.
// A leaf can be recorded at any number of entries, including no entries (in which case an empty slice must be returned).
type MapFn func([]byte) [][32]byte

// InputLog represents a connection to the input log from which map data will be built.
// This can be a local or remote data source.
type InputLog interface {
	// GetCheckpoint returns the latest checkpoint committing to the input log state.
	GetCheckpoint(ctx context.Context) (checkpoint []byte, err error)
	// StreamLeaves returns all the leaves in the range [start, end), outputting them via
	// the out channel.
	// TODO(mhutchinson): This out channel would be better as a returned iterator.
	StreamLeaves(ctx context.Context, start, end uint64, out chan<- InputLeaf)
}

// InputLeaf represents a single leaf in the input log.
type InputLeaf struct {
	// Leaf is the raw data stored at this leaf.
	Leaf []byte
	// Error contains any error fetching this leaf, and should be checked before Leaf.
	Error error
}

// LogParseFn is a function that parses a checkpoint, validating it, and returns a parsed
// checkpoint. This is expected to be a thin wrapper around log.ParseCheckpoint with the
// validators set up according to the index operator's policy on the number of witnesses
// required.
type LogParseFn func(cpRaw []byte) (*log.Checkpoint, error)

// NewVerifiableIndex returns an IndexBuilder that pulls entries from the given inputLog, determines
// indices for each one using the mapFn, and then writes the entries out to a Write Ahead Log at the given
// path.
// Note that only one IndexBuilder should exist for any given walPath at any time. The behaviour is unspecified,
// but likely broken, if multiple processes are writing to the same file at any given time.
func NewVerifiableIndex(ctx context.Context, inputLog InputLog, logParseFn LogParseFn, mapFn MapFn, walPath string) (*VerifiableIndex, error) {
	wal := &writeAheadLog{
		walPath: walPath,
	}
	ws, err := wal.init()
	if err != nil {
		return nil, err
	}
	reader, err := newLogReader(walPath)
	if err != nil {
		return nil, err
	}
	vtreeStorage := mpt.NewMemoryStorage()
	if err := mpt.InitStorage(sha256.Sum256, vtreeStorage); err != nil {
		return nil, fmt.Errorf("InitStorage: %s", err)
	}
	b := &VerifiableIndex{
		inputLog:   inputLog,
		logParseFn: logParseFn,
		mapFn:      mapFn,
		wal:        wal,
		reader:     reader,
		vindex:     *mpt.NewTree(sha256.Sum256, vtreeStorage),
		data:       map[[32]byte][]uint64{},
		nextIndex:  ws,
	}
	return b, nil
}

// VerifiableIndex manages reading from the input log, mapping leaves, updating the WAL,
// reading the WAL, and keeping the state of the in-memory index updated from the WAL.
type VerifiableIndex struct {
	inputLog   InputLog
	logParseFn LogParseFn
	mapFn      MapFn
	wal        *writeAheadLog
	reader     *logReader

	indexMu sync.RWMutex // covers vindex and data
	vindex  mpt.Tree
	data    map[[32]byte][]uint64

	nextIndex uint64 // nextIndex is the next index in the log to consume
	rawCp     []byte // rawCp is the last checkpoint we started syncing to
	cpSize    uint64 // cpSize is the tree size of rawCp
	mapSize   uint64 // mapSize is the last index from the log that was put into the map

	// servingSize is the size of the input log we are serving for.
	// This a temporary workaround not having an output log, which we will eventually read to get
	// the checkpoint size.
	servingSize uint64
}

// Close ensures that any open connections are closed before returning.
func (b *VerifiableIndex) Close() error {
	return b.wal.close()
}

// Lookup returns the values stored for the given key.
// TODO(mhutchinson): This needs to return verifiable stuff
func (b *VerifiableIndex) Lookup(key string) (indices []uint64) {
	// Scope the lock to be as minimal as possible
	lookupLocked := func(key string) []uint64 {
		b.indexMu.RLock()
		defer b.indexMu.RUnlock()
		kh := sha256.Sum256([]byte(key))
		return b.data[kh]
	}

	// TODO(mhutchinson): this should come from the latest map root in the (witnessed) output log.
	// This map root, the witnessed output log checkpoint, and all proofs should also be served here.
	size := b.servingSize

	allIndices := lookupLocked(key)
	for i, idx := range allIndices {
		if idx >= size {
			// If we have indices past the current size we are serving, drop them.
			// Doing this allows us to update b.data with new indices while still serving from it.
			return allIndices[:i]
		}
	}
	return allIndices
}

// Update checks the input log for a new Checkpoint, and ensures that the Verifiable Index
// is updated to the corresponding size.
func (b *VerifiableIndex) Update(ctx context.Context) error {
	rawCp, err := b.inputLog.GetCheckpoint(ctx)
	if err != nil {
		return fmt.Errorf("failed to get latest checkpoint from DB: %s", err)
	}
	cp, err := b.logParseFn(rawCp)
	if err != nil {
		return fmt.Errorf("failed to parse checkpoint: %s", err)
	}

	if cp.Size == b.cpSize {
		klog.V(1).Infof("No update needed: checkpoint size is still %d", b.servingSize)
		return nil
	}
	b.cpSize = cp.Size
	b.rawCp = rawCp
	klog.Infof("Building map to log size of %d", b.cpSize)

	eg, cctx := errgroup.WithContext(ctx)
	eg.Go(func() error { return b.syncFromInputLog(cctx) })
	eg.Go(func() error { return b.buildMap(cctx) })

	b.servingSize = b.cpSize

	return eg.Wait()
}

// syncFromInputLog reads the latest checkpoint from the input log, and ensures that the WAL
// contains a corresponding entry for every index committed to by that checkpoint.
//
// TODO(mhutchinson): this doesn't perform any validation on the input log to check the
// leaves correspond to the checkpoint root hash. This was reasonable while it was based on the
// cloneDB, which performed this validation. Implementing this will require the index to store some
// state alongside the WAL which contains a compact range of its current progress.
func (b *VerifiableIndex) syncFromInputLog(ctx context.Context) error {
	if b.cpSize > b.nextIndex {
		leaves := make(chan InputLeaf, 1)
		go b.inputLog.StreamLeaves(ctx, b.nextIndex, b.cpSize, leaves)

		var l InputLeaf
		for ; b.nextIndex < b.cpSize; b.nextIndex++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case l = <-leaves:
			}
			if err := l.Error; err != nil {
				return fmt.Errorf("failed to read leaf at index %d: %v", b.nextIndex, err)
			}
			var hashes [][32]byte
			var mapErr error
			func() {
				defer func() {
					if r := recover(); r != nil {
						mapErr = fmt.Errorf("panic detected mapping index %d: %s", b.nextIndex, r)
					}
				}()
				hashes = b.mapFn(l.Leaf)
			}()
			if mapErr != nil {
				return mapErr
			}
			if len(hashes) == 0 && b.nextIndex < b.cpSize-1 {
				// We can skip writing out values with no hashes, as long as we're
				// not at the end of the log.
				// If we are at the end of the log, we need to write out a value as a sentinel
				// even if there are no hashes.
				continue
			}
			if err := b.wal.append(b.nextIndex, hashes); err != nil {
				return fmt.Errorf("failed to add index to entry for leaf %d: %v", b.nextIndex, err)
			}
		}
	}
	return nil
}

// buildMap reads from the WAL until the file has been consumed and the map has been
// built up the WAL size.
func (b *VerifiableIndex) buildMap(ctx context.Context) error {
	startWal := time.Now()
	updatedKeys := make(map[[32]byte]bool) // Allows us to efficiently update vindex after first init
	for b.mapSize < b.cpSize-1 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		idx, hashes, err := b.reader.next()
		if err != nil {
			if err != io.EOF {
				return err
			}
			// Wait a small amount of time for more data to become available
			time.Sleep(10 * time.Millisecond)
			continue
		}

		// Locking strategy when updating the map containing actual data is to lock for each log entry
		// This more granular locking allows Lookup to still occur, and we can drop any indexes bigger than
		// the tree size.
		func() {
			b.indexMu.Lock()
			defer b.indexMu.Unlock()
			b.mapSize = idx
			for _, h := range hashes {
				klog.V(2).Infof("Read from WAL: index %d: %x", idx, h)
				// Add the data to the key/value map
				idxes := b.data[h]
				idxes = append(idxes, idx)
				b.data[h] = idxes
				updatedKeys[h] = true
			}
		}()
	}
	durationWal := time.Since(startWal)

	startVIndex := time.Now()
	// Build the verifiable index _afterwards_ for several reasons:
	//  1) doing this incrementally leads to a lot of duplicate work for keys with multiple values
	//  2) updating the vindex needs to block lookups for the whole update of the data structure

	// Locking strategy for the verifiable index is to prevent all reads while this is being updated.
	// TODO(mhutchinson): inside the same mutex we will need to update the output log with the calculated
	// map root, and eventually witness checkpoints.
	// If this is too slow (almost certain), then we need some strategy to allow us to serve a version of
	// the vindex while also updating it. One approach could be to have 2 trees whenever we are performing
	// an update.
	b.indexMu.Lock()
	defer b.indexMu.Unlock()
	for h := range updatedKeys {
		idxes := b.data[h]

		// Here we hash by simply appending all indices in the list and hashing that
		// TODO(mhutchinson): maybe use a log construction?
		sum := sha256.New()
		for _, idx := range idxes {
			if err := binary.Write(sum, binary.BigEndian, idx); err != nil {
				klog.Warning(err)
				return err
			}
		}

		// Finally, we update the vindex
		if err := b.vindex.Insert(h, [32]byte(sum.Sum(nil))); err != nil {
			return fmt.Errorf("Insert(): %s", err)
		}
	}
	durationVIndex := time.Since(startVIndex)
	durationTotal := time.Since(startWal)

	klog.Infof("buildMap: total=%s (wal=%s, vindex=%s)", durationTotal, durationWal, durationVIndex)
	return nil
}

type writeAheadLog struct {
	walPath string
	f       *os.File
}

// init verifies that the log is in good shape, and returns the index that is expected next.
// It also opens the log for appending to.
//
// Note that it returns the next expected index to avoid awkwardness with the meaning of 0,
// which could mean 0 was successfully read from a previous run, or that there was no log.
func (l *writeAheadLog) init() (uint64, error) {
	ffs := os.O_WRONLY | os.O_APPEND

	idx, err := l.validate()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return idx, err
		}
		ffs |= os.O_CREATE | os.O_EXCL
	} else {
		// If the file exists, then we expect the next index to be returned
		idx++
	}
	// Open the file for writing in append-only, creating it if needed
	l.f, err = os.OpenFile(l.walPath, ffs, 0o644)
	if err != nil {
		return 0, fmt.Errorf("failed to open file for writing: %s", err)
	}
	return idx, err
}

func (l *writeAheadLog) close() error {
	return l.f.Close()
}

// validate reads the file and determines what the last mapped log index was, and returns it.
// The assumption is that all lines ending with a newline were written correctly.
// If there are any errors in the file then this throws an error.
func (l *writeAheadLog) validate() (uint64, error) {
	f, err := os.Open(l.walPath)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = f.Close()
	}()
	fi, err := f.Stat()
	if err != nil {
		return 0, err
	}

	// Handle trivial case of empty file
	size := fi.Size()
	if size == 0 {
		if err := os.Remove(l.walPath); err != nil {
			return 0, fmt.Errorf("failed to delete empty file: %s", err)
		}
		return 0, os.ErrNotExist
	}

	// Confirm last character is a newline
	// TODO(mhutchinson): support ignoring incomplete lines
	lastChar := make([]byte, 1)
	if _, err := f.ReadAt(lastChar, size-1); err != nil {
		return 0, err
	}
	if lastChar[0] != '\n' {
		return 0, fmt.Errorf("expected final newline but got '%x'", lastChar[0])
	}

	// Read from the end of the file in stripes, terminating when we either:
	// a) find another newline; or
	// b) we have read from the beginning of the file
	var lastLine string
	const stripeSize = 1024
	readStripe := make([]byte, stripeSize)
	// Set it up so we read all but the last character (we know it's a newline)
	currOffset := size - 1 - stripeSize

	for {
		if currOffset < 0 {
			// If the stripe is bigger than the remaining file contents, adjust the offset
			// and scale down what we'll read to avoid reading duplicates.
			readStripe = readStripe[:stripeSize+currOffset]
			currOffset = 0
		}
		if _, err := f.ReadAt(readStripe, currOffset); err != nil {
			return 0, err
		}
		lastLine = string(readStripe) + lastLine
		if idx := strings.LastIndexByte(lastLine, '\n'); idx > 0 {
			lastLine = lastLine[idx+1:]
			break
		}
		if currOffset == 0 {
			// We read from the start of the file so lastLine is full
			break
		}
		currOffset = currOffset - stripeSize
	}

	idx, _, err := unmarshalWalEntry(lastLine)

	return idx, err
}

func (l *writeAheadLog) append(idx uint64, hashes [][32]byte) error {
	e, err := marshalWalEntry(idx, hashes)
	if err != nil {
		return fmt.Errorf("failed to marshal entry: %v", err)
	}
	_, err = l.f.WriteString(fmt.Sprintf("%s\n", e))
	return err
}

func newLogReader(path string) (*logReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &logReader{
		f: f,
		r: bufio.NewReader(f),
	}, nil
}

type logReader struct {
	f       *os.File
	r       *bufio.Reader
	partial string
}

// next returns the next index, hashes, and any error.
// TODO(mhutchinson): change this as it's inconvenient with EOF handling,
// which should be common when reader hits the end of the file but more is
// to be written.
func (r *logReader) next() (uint64, [][32]byte, error) {
	line, err := r.r.ReadString('\n')
	if err != nil {
		if err == io.EOF {
			r.partial = line
		}
		return 0, nil, err
	}

	// Make sure any partial lines are prepended, and drop the final newline
	line = r.partial + line[:len(line)-1]
	r.partial = ""
	return unmarshalWalEntry(line)
}

func (r *logReader) close() error {
	return r.f.Close()
}

// unmarshalWalEntry parses a line from the WAL.
// This is the reverse of marshalWalEntry.
func unmarshalWalEntry(e string) (uint64, [][32]byte, error) {
	tokens := strings.Split(e, " ")
	idx, err := strconv.ParseUint(tokens[0], 10, 64)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to parse idx from %q", e)
	}

	hashes := make([][32]byte, 0, len(tokens)-1)
	for i, h := range tokens[1:] {
		parsed, err := hex.DecodeString(h)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to parse hex token %d from %q", i, e)
		}
		if got, want := len(parsed), 32; got != want {
			return 0, nil, fmt.Errorf("expected 32 byte hash but got %d bytes at idx %d", got, i)
		}
		hashes = append(hashes, [32]byte(parsed))
	}

	return idx, hashes, nil
}

// unmarshalWalEntry converts an index and the hashes it affects into a line for the WAL.
// This is the reverse of unmarshalWalEntry.
func marshalWalEntry(idx uint64, hashes [][32]byte) (string, error) {
	sb := strings.Builder{}
	if _, err := sb.WriteString(strconv.FormatUint(idx, 10)); err != nil {
		return "", err
	}
	for _, h := range hashes {
		if _, err := sb.WriteString(" " + hex.EncodeToString(h[:])); err != nil {
			return "", err
		}
	}
	return sb.String(), nil
}
