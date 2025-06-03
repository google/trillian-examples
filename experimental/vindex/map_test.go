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
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
)

func TestWriteAheadLog_init(t *testing.T) {
	testCases := []struct {
		desc         string
		fileContents string
		wantIdx      uint64
		wantErr      bool
	}{
		{
			desc:         "empty file",
			fileContents: "",
			wantIdx:      0,
			wantErr:      false,
		}, {
			desc:         "0 file",
			fileContents: "0\n",
			wantIdx:      1,
			wantErr:      false,
		}, {
			desc:         "just indexes",
			fileContents: "0\n1\n2\n",
			wantIdx:      3,
			wantErr:      false,
		}, {
			desc:         "indexes and hashes",
			fileContents: "1 deadbeef feed0124\n",
			wantIdx:      2,
			wantErr:      false,
		}, {
			desc:         "trailing corruption",
			fileContents: "1\n2 fdfxx",
			wantErr:      true,
		}, {
			desc:         "lots of newlines",
			fileContents: "1\n2\n3\n\n",
			wantErr:      true,
		}, {
			desc:         "no trailing newlines",
			fileContents: "1\n2\n3",
			wantErr:      true,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			f, err := os.CreateTemp("", "testWal")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.WriteString(tC.fileContents); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			wal := &writeAheadLog{
				walPath: f.Name(),
			}
			idx, err := wal.init()
			if gotErr := err != nil; gotErr != tC.wantErr {
				t.Fatalf("wantErr != gotErr (%t != %t) %v", tC.wantErr, gotErr, err)
			}
			defer func() {
				_ = wal.close()
			}()
			if tC.wantErr {
				return
			}
			if idx != tC.wantIdx {
				t.Errorf("want idx %v but got %v", tC.wantIdx, idx)
			}
		})
	}
}

func TestWriteAheadLog_roundtrip(t *testing.T) {
	f, err := os.CreateTemp("", "testWal")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(f.Name()); err != nil {
		t.Fatal(err)
	}

	wal := &writeAheadLog{
		walPath: f.Name(),
	}
	idx, err := wal.init()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := idx, uint64(0); got != want {
		t.Fatalf("expected index %d, got %d", want, got)
	}

	for i := range 33 {
		hash := sha256.Sum256([]byte{byte(i)})
		wal.append(uint64(i), [][]byte{hash[:]})
	}

	if err := wal.close(); err != nil {
		t.Error(err)
	}

	idx, err = wal.init()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := idx, uint64(33); got != want {
		t.Fatalf("expected index %d, got %d", want, got)
	}

	if err := wal.close(); err != nil {
		t.Error(err)
	}
}

func TestWriteAndWriteLog(t *testing.T) {
	f, err := os.CreateTemp("", "testWal")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(f.Name()); err != nil {
		t.Fatal(err)
	}

	wal := &writeAheadLog{
		walPath: f.Name(),
	}
	idx, err := wal.init()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := idx, uint64(0); got != want {
		t.Fatalf("expected index %d, got %d", want, got)
	}

	reader, err := newLogReader(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	const count = 2056
	var eg errgroup.Group
	eg.Go(func() error {
		for i := range count {
			hash := sha256.Sum256([]byte{byte(i)})
			wal.append(uint64(i), [][]byte{hash[:]})
		}
		return nil
	})
	eg.Go(func() error {
		var expect uint64
		for expect < count {
			idx, _, err := reader.next()
			if err != nil {
				if err != io.EOF {
					return err
				}
				// Wait a small amount of time for more data to become available
				time.Sleep(10 * time.Millisecond)
				continue
			}
			if got, want := idx, expect; got != want {
				return fmt.Errorf("expected index %d, got %d", want, got)
			}
			expect++
		}
		return nil
	})
	if err := eg.Wait(); err != nil {
		t.Fatal(err)
	}

	if err := wal.close(); err != nil {
		t.Error(err)
	}
	if err := reader.close(); err != nil {
		t.Error(err)
	}
}

func TestUnmarshal(t *testing.T) {
	testCases := []struct {
		desc       string
		entry      string
		wantErr    bool
		wantIdx    uint64
		wantHashes int
	}{
		{
			desc:       "just index",
			entry:      "1",
			wantErr:    false,
			wantIdx:    1,
			wantHashes: 0,
		}, {
			desc:       "index and hashes",
			entry:      "1 deadbeef feed0124",
			wantErr:    false,
			wantIdx:    1,
			wantHashes: 2,
		}, {
			desc:    "corruption at the end",
			entry:   "1 deadbeef feed01xxx",
			wantErr: true,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			idx, hashes, err := unmarshalWalEntry(tC.entry)
			if gotErr := err != nil; gotErr != tC.wantErr {
				t.Fatalf("wantErr != gotErr (%t != %t) %v", tC.wantErr, gotErr, err)
			}
			if tC.wantErr {
				return
			}
			if idx != tC.wantIdx {
				t.Errorf("want idx %v but got %v", tC.wantIdx, idx)
			}
			if got, want := len(hashes), tC.wantHashes; got != want {
				t.Errorf("want %v hashes but got %v: %q", want, got, hashes)
			}
		})
	}
}
