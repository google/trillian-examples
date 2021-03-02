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

package fs

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestSeqPath(t *testing.T) {
	for _, test := range []struct {
		root     string
		seq      uint64
		wantDir  string
		wantFile string
	}{
		{
			root:     "/root/path",
			seq:      0,
			wantDir:  "/root/path/seq/00/00/00/00",
			wantFile: "00",
		}, {
			root:     "/root/path",
			seq:      0x85,
			wantDir:  "/root/path/seq/00/00/00/00",
			wantFile: "85",
		}, {
			root:     "/a/different/root/path",
			seq:      0x86,
			wantDir:  "/a/different/root/path/seq/00/00/00/00",
			wantFile: "86",
		}, {
			root:     "/a/different/root/path",
			seq:      0xffeeddccbb,
			wantDir:  "/a/different/root/path/seq/ff/ee/dd/cc",
			wantFile: "bb",
		},
	} {
		desc := fmt.Sprintf("root %q seq %d", test.root, test.seq)
		t.Run(desc, func(t *testing.T) {
			gotDir, gotFile := seqPath(test.root, test.seq)
			if gotDir != test.wantDir {
				t.Errorf("Got dir %q want %q", gotDir, test.wantDir)
			}
			if gotFile != test.wantFile {
				t.Errorf("got file %q want %q", gotFile, test.wantFile)
			}
		})
	}
}

func TestLeafPath(t *testing.T) {
	for _, test := range []struct {
		root     string
		hash     []byte
		wantDir  string
		wantFile string
	}{
		{
			root:     "/root/path",
			hash:     []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77},
			wantDir:  "/root/path/leaves/11/22/33",
			wantFile: "44556677",
		}, {
			root:     "/root/path",
			hash:     []byte{0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd},
			wantDir:  "/root/path/leaves/88/99/aa",
			wantFile: "bbccdd",
		}, {
			root:     "/a/different/root/path",
			hash:     []byte{0x12, 0x34, 0x56, 0x78, 0x9a},
			wantDir:  "/a/different/root/path/leaves/12/34/56",
			wantFile: "789a",
		},
	} {
		desc := fmt.Sprintf("root %q hash %x", test.root, test.hash)
		t.Run(desc, func(t *testing.T) {
			gotDir, gotFile := leafPath(test.root, test.hash)
			if gotDir != test.wantDir {
				t.Errorf("Got dir %q want %q", gotDir, test.wantDir)
			}
			if gotFile != test.wantFile {
				t.Errorf("got file %q want %q", gotFile, test.wantFile)
			}
		})
	}
}

func TestTilePath(t *testing.T) {
	for _, test := range []struct {
		root     string
		level    uint64
		index    uint64
		wantDir  string
		wantFile string
	}{
		{
			root:     "/root/path",
			level:    0,
			index:    0,
			wantDir:  "/root/path/tile/00/0000/00/00",
			wantFile: "00",
		}, {
			root:     "/root/path",
			level:    0x10,
			index:    0,
			wantDir:  "/root/path/tile/10/0000/00/00",
			wantFile: "00",
		}, {
			root:     "/root/path",
			level:    0x10,
			index:    0x455667,
			wantDir:  "/root/path/tile/10/0000/45/56",
			wantFile: "67",
		}, {
			root:     "/root/path",
			level:    0x10,
			index:    0x123456789a,
			wantDir:  "/root/path/tile/10/1234/56/78",
			wantFile: "9a",
		}, {
			root:     "/a/different/root/path",
			level:    0x15,
			index:    0x455667,
			wantDir:  "/a/different/root/path/tile/15/0000/45/56",
			wantFile: "67",
		},
	} {
		desc := fmt.Sprintf("root %q level %x index %x", test.root, test.level, test.index)
		t.Run(desc, func(t *testing.T) {
			gotDir, gotFile := tilePath(test.root, test.level, test.index)
			if gotDir != test.wantDir {
				t.Errorf("Got dir %q want %q", gotDir, test.wantDir)
			}
			if gotFile != test.wantFile {
				t.Errorf("got file %q want %q", gotFile, test.wantFile)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	empty := []byte("empty")

	d := filepath.Join(t.TempDir(), "storage")
	s, err := Create(d, empty)
	if err != nil {
		t.Fatalf("Create = %v", err)
	}

	ls := s.LogState()
	if got, want := ls.Size, uint64(0); got != want {
		t.Errorf("New logstate has size %d, want %d", got, want)
	}
	if got, want := ls.RootHash, empty; !bytes.Equal(got, want) {
		t.Errorf("New logstate roothash %x, want %x", got, want)
	}
	if got, want := len(ls.Hashes), 0; got != want {
		t.Errorf("New logstate hashes is size %d, want %d", got, want)
	}
}

func TestCreateForExistingDirectory(t *testing.T) {
	// This dir will already exist since the test framework just created it.
	d := t.TempDir()

	_, err := Create(d, []byte("empty"))
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("Create = %v, want already exists error", err)
	}
}

func TestNew(t *testing.T) {
	empty := []byte("empty")

	d := filepath.Join(t.TempDir(), "storage")
	_, err := Create(d, empty)
	if err != nil {
		t.Fatalf("Create = %v", err)
	}

	if _, err := New(d); err != nil {
		t.Fatalf("New = %v, want no error", err)
	}
}

func TestNewForNonExistentDir(t *testing.T) {
	if _, err := New("5oi4egdf93uyjigedfk"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("New = %v, want not exists error", err)
	}
}

func TestNewWithCorruptState(t *testing.T) {
	empty := []byte("empty")

	d := filepath.Join(t.TempDir(), "storage")
	_, err := Create(d, empty)
	if err != nil {
		t.Fatalf("Create = %v", err)
	}

	if err := ioutil.WriteFile(filepath.Join(d, statePath), []byte("][bananas!"), 0644); err != nil {
		t.Fatalf("Failed to write corrupt log state file; %q", err)
	}

	if _, err := New(d); err == nil {
		t.Fatal("New = nil, want err")
	}
}
func TestUpdateState(t *testing.T) {
	empty := []byte("empty")

	d := filepath.Join(t.TempDir(), "storage")
	s, err := Create(d, empty)
	if err != nil {
		t.Fatalf("Create = %v", err)
	}

	ls := s.LogState()
	ls.Size++

	if err := s.UpdateState(ls); err != nil {
		t.Fatalf("UpdateState = %v", err)
	}

	ls2 := s.LogState()

	if diff := cmp.Diff(ls2, ls); len(diff) != 0 {
		t.Errorf("Updated state had diff %s", diff)
	}
}
