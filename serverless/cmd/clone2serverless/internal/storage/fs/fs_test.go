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
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/transparency-dev/serverless/pkg/log"
)

func TestCreate(t *testing.T) {
	d := filepath.Join(t.TempDir(), "storage")
	_, err := Create(d)
	if err != nil {
		t.Fatalf("Create = %v", err)
	}
}

func TestCreateForExistingDirectory(t *testing.T) {
	// This dir will already exist since the test framework just created it.
	d := t.TempDir()

	_, err := Create(d)
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("Create = %v, want already exists error", err)
	}
}

func TestLoad(t *testing.T) {
	d := filepath.Join(t.TempDir(), "storage")
	_, err := Create(d)
	if err != nil {
		t.Fatalf("Create = %v", err)
	}

	if _, err := Load(d, 0); err != nil {
		t.Fatalf("Load = %v, want no error", err)
	}
}

func TestLoadForNonExistentDir(t *testing.T) {
	if _, err := Load("5oi4egdf93uyjigedfk", 0); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load = %v, want not exists error", err)
	}
}

func TestWriteLoadState(t *testing.T) {
	d := filepath.Join(t.TempDir(), "storage")
	s, err := Create(d)
	if err != nil {
		t.Fatalf("Create = %v", err)
	}

	a := []byte("hello")

	if err := s.WriteCheckpoint(context.Background(), a); err != nil {
		t.Fatalf("WriteCheckpoint = %v", err)
	}

	b, err := ReadCheckpoint(d)
	if err != nil {
		t.Fatalf("ReadCheckpoint = %v", err)
	}

	if diff := cmp.Diff(b, a); len(diff) != 0 {
		t.Errorf("Updated checkpoint had diff %s", diff)
	}
}

type errCheck func(error) bool

func TestSequence(t *testing.T) {
	ctx := context.Background()

	for _, test := range []struct {
		desc    string
		leaves  [][]byte
		wantSeq []uint64
		wantErr []errCheck
	}{
		{
			desc:    "sequences ok",
			leaves:  [][]byte{{0x00}, {0x01}, {0x02}},
			wantSeq: []uint64{0, 1, 2},
		}, {
			desc:    "dupe squashed",
			leaves:  [][]byte{{0x10}, {0x10}},
			wantSeq: []uint64{0, 0},
			wantErr: []errCheck{nil, func(e error) bool { return errors.Is(e, log.ErrDupeLeaf) }},
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			d := filepath.Join(t.TempDir(), "storage")
			s, err := Create(d)
			if err != nil {
				t.Fatalf("Create = %v", err)
			}
			for i, leaf := range test.leaves {
				h := sha256.Sum256(leaf)
				gotSeq, gotErr := s.Sequence(ctx, h[:], leaf)
				if gotErr != nil {
					t.Logf("Sequence %d = %v", i, gotErr)
				}
				if gotErr != nil && test.wantErr[i] == nil {
					t.Errorf("Got unexpected error %v, want no error", gotErr)
				}
				if test.wantErr != nil && test.wantErr[i] != nil && !test.wantErr[i](gotErr) {
					t.Errorf("Got wrong type of error %T (%[1]q)", gotErr)
				}
				if gotSeq != test.wantSeq[i] {
					t.Fatalf("Got sequence number %d, want %d", gotSeq, test.wantSeq[i])
				}
			}
		})
	}
}

func TestAssign(t *testing.T) {
	ctx := context.Background()

	type leafSeq struct {
		leaf    []byte
		seq     uint64
		wantErr bool
	}
	for _, test := range []struct {
		desc   string
		leaves []leafSeq
	}{
		{
			desc: "assigns ok",
			leaves: []leafSeq{
				{
					leaf: []byte{0x00},
					seq:  0,
				}, {
					leaf: []byte{0x01},
					seq:  1,
				}, {
					leaf: []byte{0x02},
					seq:  2,
				},
			},
		}, {
			desc: "assigns non-sequential ok",
			leaves: []leafSeq{
				{
					leaf: []byte{0x00},
					seq:  1,
				}, {
					leaf: []byte{0x01},
					seq:  0,
				}, {
					leaf: []byte{0x02},
					seq:  2,
				},
			},
		}, {
			desc: "duplicate seq",
			leaves: []leafSeq{
				{
					leaf: []byte{0x00},
					seq:  0,
				}, {
					leaf: []byte{0x01},
					seq:  1,
				}, {
					leaf:    []byte{0x02},
					seq:     1, // duplicate
					wantErr: true,
				},
			},
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			d := filepath.Join(t.TempDir(), "storage")
			s, err := Create(d)
			if err != nil {
				t.Fatalf("Create = %v", err)
			}
			for _, ls := range test.leaves {
				err := s.Assign(ctx, ls.seq, ls.leaf)
				if err != nil {
					t.Logf("Assign(%d, ...) = %v", ls.seq, err)
				}
				if gotErr := err != nil; gotErr != ls.wantErr {
					t.Errorf("Got unexpected error %v, want no error", err)
				}
			}
		})
	}

}
