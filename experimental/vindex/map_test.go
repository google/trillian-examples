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
	"os"
	"testing"
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
			desc:         "just indexes",
			fileContents: "0\n1\n2\n",
			wantIdx:      2,
			wantErr:      false,
		}, {
			desc:         "indexes and hashes",
			fileContents: "1 deadbeef feed0124\n",
			wantIdx:      1,
			wantErr:      false,
		}, {
			desc:         "trailing corruption",
			fileContents: "1\n2 fdfxx",
			wantIdx:      1,
			wantErr:      false,
		}, {
			desc:         "lots of newlines",
			fileContents: "1\n2\n3\n\n",
			wantIdx:      3,
			wantErr:      false,
		}, {
			desc:         "no trailing newlines",
			fileContents: "1\n2\n3",
			wantIdx:      3,
			wantErr:      false,
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
			if tC.wantErr {
				return
			}
			if idx != tC.wantIdx {
				t.Errorf("want idx %v but got %v", tC.wantIdx, idx)
			}
		})
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
