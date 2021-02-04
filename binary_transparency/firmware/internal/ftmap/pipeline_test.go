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

package ftmap

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/apache/beam/sdks/go/pkg/beam"
	"github.com/apache/beam/sdks/go/pkg/beam/testing/passert"
	"github.com/apache/beam/sdks/go/pkg/beam/testing/ptest"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian/experimental/batchmap"
)

func TestCreate(t *testing.T) {
	tests := []struct {
		name   string
		treeID int64
		count  int64

		wantRoot string
		wantLogs []string
	}{
		{
			name:   "One entry",
			treeID: 12345,
			count:  1,

			wantRoot: "85749402df8a992bbce4d3cd5a8205421b73dcc6df860253ddcbceddf60ed903",
			wantLogs: []string{"dummy: [1]"},
		},
		{
			name:   "All entries",
			treeID: 12345,
			count:  -1,

			wantRoot: "eb994efa95861a85b0a463f1499af81c41370882abb715e456ba79813f3a88d2",
			wantLogs: []string{"dummy: [1 5 3]", "fish: [42]"},
		},
	}

	inputLog := fakeLog{
		leaves: []api.FirmwareMetadata{
			{
				DeviceID:         "dummy",
				FirmwareRevision: 1,
			},
			{
				DeviceID:         "dummy",
				FirmwareRevision: 5,
			},
			{
				DeviceID:         "fish",
				FirmwareRevision: 42,
			},
			{
				DeviceID:         "dummy",
				FirmwareRevision: 3,
			},
		},
		head: []byte("this is just passed around"),
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			mb := NewMapBuilder(inputLog, test.treeID, 0)
			p, s := beam.NewPipelineWithRoot()

			tiles, logs, _, err := mb.Create(s, test.count)
			if err != nil {
				t.Errorf("failed to Create(): %v", err)
			}

			rootToString := func(t *batchmap.Tile) string { return fmt.Sprintf("%x", t.RootHash) }
			passert.Equals(s, beam.ParDo(s, rootToString, tiles), test.wantRoot)

			logToString := func(l *DeviceReleaseLog) string { return fmt.Sprintf("%s: %v", l.DeviceID, l.Revisions) }
			passert.Equals(s, beam.ParDo(s, logToString, logs), beam.CreateList(s, test.wantLogs))

			err = ptest.Run(p)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

type fakeLog struct {
	leaves []api.FirmwareMetadata
	head   []byte
}

func (l fakeLog) Head() ([]byte, int64, error) {
	return l.head, int64(len(l.leaves)), nil
}

func (l fakeLog) Entries(s beam.Scope, start, end int64) beam.PCollection {
	count := int(end - start)
	entries := make([]InputLogLeaf, count)
	for i := 0; i < count; i++ {
		// This swallows the error, but the test will fail anyway. YOLO.
		index := start + int64(i)
		fwbs, _ := json.Marshal(l.leaves[int(index)])
		bs, _ := json.Marshal(api.SignedStatement{
			Type:      api.FirmwareMetadataType,
			Statement: fwbs,
		})
		entries[i] = InputLogLeaf{
			Seq:  index,
			Data: bs,
		}
	}
	return beam.CreateList(s, entries)
}
