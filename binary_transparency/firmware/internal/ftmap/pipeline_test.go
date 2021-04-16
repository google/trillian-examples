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
	"crypto/sha512"
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

			wantRoot: "865bbd0034750dd6abe512470722fad51c85ba95d96316818e8ec8a2a55679c0",
			wantLogs: []string{"dummy: [1]"},
		},
		{
			name:   "All entries",
			treeID: 12345,
			count:  -1,

			wantRoot: "daa0e0c66d69162abbe27ba9aa54a8bdb8850f1100e0626e45ea477cea4765e6",
			wantLogs: []string{"dummy: [1 5 3]", "fish: [42]"},
		},
	}

	inputLog := fakeLog{
		leaves: []api.SignedStatement{
			createFWSignedStatement("dummy", 1),
			createFWSignedStatement("dummy", 5),
			createFWSignedStatement("fish", 42),
			createFWSignedStatement("dummy", 3),
		},
		head: []byte("this is just passed around"),
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			mb := NewMapBuilder(inputLog, test.treeID, 0)
			p, s := beam.NewPipelineWithRoot()

			result, err := mb.Create(s, test.count)
			if err != nil {
				t.Fatalf("failed to Create(): %v", err)
			}

			rootToString := func(t *batchmap.Tile) string { return fmt.Sprintf("%x", t.RootHash) }
			passert.Equals(s, beam.ParDo(s, rootToString, result.MapTiles), test.wantRoot)

			logToString := func(l *api.DeviceReleaseLog) string { return fmt.Sprintf("%s: %v", l.DeviceID, l.Revisions) }
			passert.Equals(s, beam.ParDo(s, logToString, result.DeviceLogs), beam.CreateList(s, test.wantLogs))

			err = ptest.Run(p)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func createFW(device string, revision uint64) api.FirmwareMetadata {
	image := fmt.Sprintf("this image is the firmware at revision %d for device %s.", revision, device)
	imageHash := sha512.Sum512([]byte(image))
	return api.FirmwareMetadata{
		DeviceID:            device,
		FirmwareRevision:    revision,
		FirmwareImageSHA512: imageHash[:],
	}
}

func createFWSignedStatement(device string, revision uint64) api.SignedStatement {
	fwbs, _ := json.Marshal(createFW(device, revision))
	return api.SignedStatement{
		Type:      api.FirmwareMetadataType,
		Statement: fwbs,
	}
}

type fakeLog struct {
	leaves []api.SignedStatement
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
		bs, _ := json.Marshal(l.leaves[int(index)])
		entries[i] = InputLogLeaf{
			Seq:  index,
			Data: bs,
		}
	}
	return beam.CreateList(s, entries)
}
