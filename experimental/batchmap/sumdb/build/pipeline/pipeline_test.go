// Copyright 2020 Google LLC. All Rights Reserved.
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

package pipeline

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/register"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/testing/passert"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/testing/ptest"
	"github.com/google/trillian/experimental/batchmap"
)

func init() {
	register.Function1x1(rootToString)
}

func rootToString(t *batchmap.Tile) string { return fmt.Sprintf("%x", t.RootHash) }

func TestCreateAndUpdateEquivalence(t *testing.T) {
	tests := []struct {
		name   string
		treeID int64
		logs   bool

		wantRoot string
	}{
		{
			name:   "No logs",
			treeID: 12345,
			logs:   false,

			wantRoot: "5d424e362148da02610565795788f3856c6d225bbfcf9963baa26abc569b6c71",
		},
	}

	inputLog := fakeLog{
		entries: []Metadata{
			{
				Module:   "foo",
				Version:  "v1.0.0",
				RepoHash: "abcdefab",
				ModHash:  "deadbeef",
			},
			{
				Module:   "bar",
				Version:  "v0.0.1",
				RepoHash: "abcdefab",
				ModHash:  "deadbeef",
			},
		},
		head: []byte("this is just passed around"),
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			mb := NewMapBuilder(inputLog, test.treeID, 0, test.logs)
			p, s := beam.NewPipelineWithRoot()

			createTiles, _, createMetadata, err := mb.Create(s, 2)
			if err != nil {
				t.Errorf("failed to Create(): %v", err)
			}

			updateTiles, _, updateMetadata, err := mb.Create(s, 1)
			if err != nil {
				t.Errorf("failed to Create(): %v", err)
			}
			updateTiles, updateMetadata, err = mb.Update(s, updateTiles, updateMetadata, 2)
			if err != nil {
				t.Errorf("failed to Update(): %v", err)
			}

			if !reflect.DeepEqual(createMetadata, updateMetadata) {
				t.Errorf("create != update (%v != %v", createMetadata, updateMetadata)
			}

			passert.Equals(s, beam.ParDo(s, rootToString, createTiles), test.wantRoot)
			passert.Equals(s, beam.ParDo(s, rootToString, updateTiles), test.wantRoot)

			err = ptest.Run(p)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

type fakeLog struct {
	entries []Metadata
	head    []byte
}

func (l fakeLog) Head() ([]byte, int64, error) {
	return l.head, int64(len(l.entries)), nil
}

func (l fakeLog) Entries(s beam.Scope, start, end int64) beam.PCollection {
	return beam.CreateList(s, l.entries)
}
