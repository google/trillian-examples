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
	"testing"

	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/testing/passert"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/testing/ptest"
)

func TestMain(m *testing.M) {
	ptest.Main(m)
}

func TestCreateEntries(t *testing.T) {
	tests := []struct {
		name     string
		treeID   int64
		metadata []Metadata

		wantCount int
	}{
		{
			name:   "single entry has 2 outputs",
			treeID: 12345,
			metadata: []Metadata{
				{
					Module:   "foo",
					Version:  "1",
					RepoHash: "abcdefab",
					ModHash:  "deadbeef",
				},
			},
			wantCount: 2,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			p, s := beam.NewPipelineWithRoot()
			metadata := beam.CreateList(s, test.metadata)

			entries := CreateEntries(s, test.treeID, metadata)

			passert.Count(s, entries, "entries", test.wantCount)
			err := ptest.Run(p)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
