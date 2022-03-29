// Copyright 2022 Google LLC. All Rights Reserved.
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

// Package slots provides a simple "postbox" type filesystem.
package slots

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
)

// magic0 is the only known journal header prefix.
const magic0 = "TFJ0"

// BlockReaderWriter describes a type which knows how to read and write
// whole blocks to some backing storage.
type BlockReaderWriter interface {
	// BlockSize returns the block size of the underlying storage system.
	BlockSize() uint

	// ReadBlocks reads len(b) bytes into b from contiguous storage blocks starting
	// at the given block address.
	// b must be an integer multiple of the device's block size.
	ReadBlocks(lba uint, b []byte) error

	// WriteBlocks writes len(b) bytes from b to contiguous storage blocks starting
	// at the given block address.
	// b must be an integer multiple of the device's block size.
	WriteBlocks(lba uint, b []byte) error
}

// Journal implements a record-based format which provides a resilient storage.
// This structure is not thread-safe, so concurrent access must be enforced at
// a higher level.
type Journal struct {
	dev          BlockReaderWriter
	start        uint
	length       uint
	current      entry
	nextBlock    uint
	maxDataBytes uint
}

// entry represents an entry in the journal.
type entry struct {
	// Magic is a 4 byte prefix which allows us to quickly filter out invalid entry records.
	// This ie expected to contain the value in the magic0 const above.
	Magic [4]byte
	// Revision is an incrementing counter which tracks the total number of successful
	// updates to a journal.
	// Each successive entry should have a revision one greater than the previous entry.
	Revision uint32
	// DataLen is the length in bytes of the application data associated with this entry.
	DataLen uint64
	// DataSHA256 is the SHA256 hash of the application data associated with this entry.
	DataSHA256 [32]byte
	// Data is the application data associated with this entry.
	Data []byte
}

const (
	// entryHeaderSize is the on-disk size of an entry without application data.
	entryHeaderSize = 4 + 4 + 8 + 32

	// minEntries is the minimum number of entries a journal must be able
	// to store.
	// At least 3 guarantees that a journal is always recoverable in the case of a
	// failed write; imagine a journal of 10 blocks where it's permitted to write
	// records of up to 50% of the available space (== 5 blocks), and that the user
	// performs 3 writes in sequence with the following sizes:
	//    1 block, 5 blocks, 5 blocks
	// but the final write fails after having only stored 2 of the 5 blocks.
	// In this case, due to the current implementation avoiding wrapping records
	// which would go past the end of the journal, we would corrupt the first two
	// entries.
	//
	// This trade-off, which currently favours simplicity over space efficiency,
	// could be shifted in the other direction by adding support for wrapping the
	// journal, at which point this value could be lowered to 2.
	minEntries = 3
)

// Size returns the number of bytes used by this entry record.
func (e *entry) Size() int {
	return entryHeaderSize + len(e.Data)
}

// OpenJournal returns a new journal structure for interacting with a journal stored in the
// [start, start+length) range of blocks accessible via dev.
// Journal ranges should not overlap with one another, or corruption will almost certainly occur.
func OpenJournal(dev BlockReaderWriter, start, length uint) (*Journal, error) {
	j := &Journal{
		dev:          dev,
		start:        start,
		length:       length,
		maxDataBytes: (length*dev.BlockSize())/minEntries - entryHeaderSize,
	}

	if err := j.init(); err != nil {
		return nil, err
	}

	return j, nil
}

// Data returns the application data from the most recent valid entry in the
// journal, along with the entry's revision number.
// If the returned revision is zero, then no successful writes have taken place
// on this journal.
func (j *Journal) Data() ([]byte, uint32) {
	return j.current.Data, j.current.Revision
}

// Update creates a new entry record in the journal for the provided data.
// The new record's revision will be one greater than the previous record (or 1
// if no previous record exists).
func (j *Journal) Update(data []byte) error {
	if l := len(data); l > int(j.maxDataBytes) {
		return fmt.Errorf("attemping to write %d bytes, larger than the max permitted in this journal (%d bytes)", l, j.maxDataBytes)
	}
	e := entry{
		Magic:      [4]byte{magic0[0], magic0[1], magic0[2], magic0[3]},
		Revision:   j.current.Revision + 1,
		DataLen:    uint64(len(data)),
		DataSHA256: sha256.Sum256(data),
		Data:       data,
	}

	cap := (j.start + j.length - j.nextBlock) * j.dev.BlockSize()
	if cap < uint(e.Size()) {
		// The record won't fit in the remaining space, so wrap around and write at the beginning.
		j.nextBlock = j.start
	}
	// TODO(al): consider making this more "streamy".
	buf := &bytes.Buffer{}
	if err := marshalEntry(e, buf); err != nil {
		return fmt.Errorf("failed to marshal entry: %v", err)
	}
	return j.dev.WriteBlocks(j.nextBlock, buf.Bytes())
}

// Init scans the journal to figure out the latest valid record, if any.
func (j *Journal) init() error {
	// Start where all good stories do: at the beginning!
	lba := j.start
	var lastEntry entry
	nextWriteLBA := j.start
	for lba < j.start+j.length {
		br := newBlockReader(j.dev, lba)
		e, err := unmarshalEntry(br)
		if err != nil {
			if lastEntry.Revision > 0 {
				// We already found the lastet record in the journal, so we're done.
				break
			}
			// Since we haven't already found a good entry, and we were unable
			// to unmarshal this one,
			// this means that either:
			//  a) the journal is completely empty, or
			//  b) the previously good entry/ies at the start of the journal
			//     have been completely or partially overwritten during a
			//     failed write attempt.
			// Either way, we don't have a valid entry wth a length field we can
			// rely on, so we we'll have to fall back to scanning all blocks to
			// look for one.
			lba++
			continue
		}
		if e.Revision > lastEntry.Revision {
			// We've found a(nother) good entry, so update our state
			lastEntry = *e
			// Skip past the blocks we've just read
			lba = (br.pos-1)/br.dev.BlockSize() + 1
			// If this turns out to be the last good entry, then we'll write
			// at the next block.
			nextWriteLBA = lba
			// But loop around again, just in case there are yet more good
			// entries following on...
			continue
		} else if e.Revision < lastEntry.Revision {
			// We've found an older revision following a newer one, so we're done.
			nextWriteLBA = lba
			break
		} else {
			return fmt.Errorf("journal is corrupt - found two entries with the same revision (%d)", e.Revision)
		}
	}
	// In the case where the very last entry in the journal is the current one,
	// and that entry extends into the final block, we'll need to wrap the
	// nextBlock pointer to the start of the journal.
	if nextWriteLBA >= j.start+j.length {
		nextWriteLBA = j.start
	}
	j.nextBlock = nextWriteLBA
	j.current = lastEntry

	return nil
}

// unmarshalEntry reads and deserialises an entry structure from the provided reader.
func unmarshalEntry(r io.Reader) (*entry, error) {
	e := &entry{}
	if err := binary.Read(r, binary.BigEndian, &e.Magic); err != nil {
		return nil, fmt.Errorf("failed to read magic: %v", err)
	}
	if string(e.Magic[:]) != magic0 {
		return nil, fmt.Errorf("invalid header magic %v", e.Magic)
	}
	if err := binary.Read(r, binary.BigEndian, &e.Revision); err != nil {
		return nil, fmt.Errorf("failed to read revision: %v", err)
	}
	if err := binary.Read(r, binary.BigEndian, &e.DataLen); err != nil {
		return nil, fmt.Errorf("failed to read data length: %v", err)
	}
	if err := binary.Read(r, binary.BigEndian, &e.DataSHA256); err != nil {
		return nil, fmt.Errorf("failed to read data SHA256: %v", err)
	}
	e.Data = make([]byte, e.DataLen)
	if _, err := io.ReadFull(r, e.Data); err != nil {
		return nil, fmt.Errorf("failed to read data; %v", err)
	}
	if h := sha256.Sum256(e.Data); !bytes.Equal(h[:], e.DataSHA256[:]) {
		return e, fmt.Errorf("incorrect data SHA256 (%x), header claims (%x)", h, e.DataSHA256[:])
	}
	return e, nil
}

// marshalEntry serialises e and writes it to the provided writer.
func marshalEntry(e entry, w io.Writer) error {
	if string(e.Magic[:]) != magic0 {
		return fmt.Errorf("invalid header magic %v", e.Magic)
	}
	if h := sha256.Sum256(e.Data); !bytes.Equal(h[:], e.DataSHA256[:]) {
		return fmt.Errorf("incorrect data SHA256 (%x), header claims (%x)", h, e.DataSHA256[:])
	}
	if err := binary.Write(w, binary.BigEndian, e.Magic); err != nil {
		return fmt.Errorf("failed to write magic: %v", err)
	}
	if err := binary.Write(w, binary.BigEndian, e.Revision); err != nil {
		return fmt.Errorf("failed to write revision: %v", err)
	}
	if err := binary.Write(w, binary.BigEndian, e.DataLen); err != nil {
		return fmt.Errorf("failed to write data length: %v", err)
	}
	if err := binary.Write(w, binary.BigEndian, e.DataSHA256); err != nil {
		return fmt.Errorf("failed to write data SHA256: %v", err)
	}
	if _, err := w.Write(e.Data); err != nil {
		return fmt.Errorf("failed to write data; %v", err)
	}
	return nil
}

// blockReader provides an io.Reader wrapper for BlockReaderWriter instances.
type blockReader struct {
	dev BlockReaderWriter
	buf []byte
	pos uint
}

// newBlockReader creates a new reader for the given BlockReaderWriter, whose Read
// function will start with the block address in lba.
func newBlockReader(dev BlockReaderWriter, lba uint) *blockReader {
	return &blockReader{
		dev: dev,
		buf: make([]byte, dev.BlockSize()),
		pos: lba * dev.BlockSize(),
	}
}

// Read implements io.Reader.
func (br *blockReader) Read(b []byte) (int, error) {
	if br.pos%br.dev.BlockSize() == 0 {
		if err := br.dev.ReadBlocks(br.pos/br.dev.BlockSize(), br.buf); err != nil {
			return 0, err
		}
	}
	l := copy(b, br.buf[br.pos%br.dev.BlockSize():])
	br.pos += uint(l)
	return l, nil
}
