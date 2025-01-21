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
// This version uses the clone tool DB as the log source.
package vindex

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/google/trillian-examples/clone/logdb"
)

// MapFn takes the raw leaf data from a log entry and outputs the SHA256 hashes
// of the keys at which this leaf should be indexed under.
// A leaf can be recorded at any number of entries, including no entries (in which case an empty slice must be returned).
type MapFn func([]byte) [][]byte

func NewIndexBuilder(ctx context.Context, log *logdb.Database, mapFn MapFn, walPath string) (IndexBuilder, error) {
	b := IndexBuilder{
		log:   log,
		mapFn: mapFn,
		wal: &writeAheadLog{
			walPath: walPath,
		},
	}
	return b, b.init(ctx)
}

type IndexBuilder struct {
	log   *logdb.Database
	mapFn MapFn
	wal   *writeAheadLog
}

func (b IndexBuilder) init(ctx context.Context) error {
	idx, err := b.wal.init()
	if err != nil {
		return err
	}

	_ = idx
	_ = ctx

	// Kick off a thread to read from the DB from the index onwards and:
	//  - update the WAL
	//  - announce new updates via a channel

	// Kick off a thread to:
	//  - read snapshot of the WAL and populate map
	//  - consume entries  from the channel to update the map

	return nil
}

type writeAheadLog struct {
	walPath string

	entries []string
}

// init reads the file and determines what the last mapped log index was, and returns it.
// This method populates entries with the lines from the WAL up to and including the last
// good entry.
func (l *writeAheadLog) init() (uint64, error) {
	f, err := os.Open(l.walPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	l.entries = make([]string, 0, 64)
	for scanner.Scan() {
		l.entries = append(l.entries, scanner.Text())
	}

	// Parse from the end, being tolerant of any corruption on final entries.
	// Any corrupt entries are dropped.
	for len(l.entries) > 0 {
		lastEntry := l.entries[len(l.entries)-1]
		idx, _, err := unmarshalWalEntry(lastEntry)
		if err == nil {
			return idx, nil
		}
		l.entries = l.entries[:len(l.entries)-1]
	}
	return 0, nil
}

// unmarshalWalEntry parses a line from the WAL.
// This is the reverse of marshalWalEntry.
func unmarshalWalEntry(e string) (uint64, [][]byte, error) {
	tokens := strings.Split(e, " ")
	idx, err := strconv.ParseUint(tokens[0], 10, 64)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to parse idx from %q", e)
	}

	hashes := make([][]byte, 0, len(tokens)-1)
	for i, h := range tokens[1:] {
		parsed, err := hex.DecodeString(h)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to parse hex token %d from %q", i, e)
		}
		hashes = append(hashes, parsed)
	}

	return idx, hashes, nil
}

// unmarshalWalEntry converts an index and the hashes it affects into a line for the WAL.
// This is the reverse of unmarshalWalEntry.
func marshalWalEntry(idx uint64, hashes [][]byte) (string, error) {
	sb := strings.Builder{}
	if _, err := sb.WriteString(strconv.FormatUint(idx, 10)); err != nil {
		return "", err
	}
	for _, h := range hashes {
		if _, err := sb.WriteString(" " + hex.EncodeToString(h)); err != nil {
			return "", err
		}
	}
	return sb.String(), nil
}
