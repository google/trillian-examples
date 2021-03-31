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
	"crypto/sha256"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian-examples/serverless/internal/storage/fs/layout"
)

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

func TestLoad(t *testing.T) {
	empty := []byte("empty")

	d := filepath.Join(t.TempDir(), "storage")
	_, err := Create(d, empty)
	if err != nil {
		t.Fatalf("Create = %v", err)
	}

	if _, err := Load(d); err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}
}

func TestLoadForNonExistentDir(t *testing.T) {
	if _, err := Load("5oi4egdf93uyjigedfk"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load = %v, want not exists error", err)
	}
}

func TestLoadWithCorruptState(t *testing.T) {
	empty := []byte("empty")

	d := filepath.Join(t.TempDir(), "storage")
	_, err := Create(d, empty)
	if err != nil {
		t.Fatalf("Create = %v", err)
	}

	if err := ioutil.WriteFile(filepath.Join(d, layout.StatePath), []byte("][bananas!"), 0644); err != nil {
		t.Fatalf("Failed to write corrupt log state file; %q", err)
	}

	if _, err := Load(d); err == nil {
		t.Fatal("Load = nil, want err")
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

func TestSequence(t *testing.T) {
	empty := []byte("empty")

	d := filepath.Join(t.TempDir(), "storage")
	s, err := Create(d, empty)
	if err != nil {
		t.Fatalf("Create = %v", err)
	}

	for _, test := range []struct {
		desc    string
		leaves  [][]byte
		wantErr bool
	}{
		{
			desc:    "sequences ok",
			leaves:  [][]byte{{0x00}, {0x01}, {0x02}},
			wantErr: false,
		}, {
			desc:    "dupe denied",
			leaves:  [][]byte{{0x10}, {0x10}},
			wantErr: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			gotErr := false
			for i, leaf := range test.leaves {
				h := sha256.Sum256(leaf)
				if err := s.Sequence(h[:], leaf); err != nil {
					t.Logf("Sequence %d = %v", i, err)
					gotErr = true
				}
			}
			if gotErr != test.wantErr {
				t.Errorf("Got error %t, want error %t", gotErr, test.wantErr)
			}
		})
	}

}
