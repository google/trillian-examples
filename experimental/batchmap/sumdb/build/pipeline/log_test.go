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
	"testing"

	"github.com/apache/beam/sdks/go/pkg/beam"
	"github.com/apache/beam/sdks/go/pkg/beam/testing/passert"
	"github.com/apache/beam/sdks/go/pkg/beam/testing/ptest"
	"github.com/google/trillian/experimental/batchmap"
)

const treeID = 12345

func TestMakeVersionLogs(t *testing.T) {
	tests := []struct {
		name     string
		metadata []Metadata

		wantCount    int
		wantRoot     string
		wantVersions []string
	}{
		{
			name: "single module single metadata",
			metadata: []Metadata{
				{
					Module:  "foo",
					Version: "v0.0.1",
					ID:      1,
				},
			},
			wantCount:    1,
			wantRoot:     "ab0fa665851a47dec50ff0a51e7dfbab747ff5a548dcfcc7bde213b571a8e6ae",
			wantVersions: []string{"v0.0.1"},
		},
		{
			name: "single module two metadata (in order)",
			metadata: []Metadata{
				{
					Module:  "foo",
					Version: "1",
					ID:      1,
				},
				{
					Module:  "foo",
					Version: "2",
					ID:      2,
				},
			},
			wantCount:    1,
			wantRoot:     "7fadb0db3926ec36f4028452856670df932eacba6f624ed82284c4a63adc5f73",
			wantVersions: []string{"1", "2"},
		},
		{
			name: "single module two metadata (out of order)",
			metadata: []Metadata{
				{
					Module:  "foo",
					Version: "2",
					ID:      2,
				},
				{
					Module:  "foo",
					Version: "1",
					ID:      1,
				},
			},
			wantCount:    1,
			wantRoot:     "7fadb0db3926ec36f4028452856670df932eacba6f624ed82284c4a63adc5f73",
			wantVersions: []string{"1", "2"},
		},
		{
			name: "two modules",
			metadata: []Metadata{
				{
					Module:  "foo",
					Version: "1",
					ID:      1,
				},
				{
					Module:  "bar",
					Version: "1",
					ID:      2,
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

			entries, logs := MakeVersionLogs(s, treeID, metadata)

			passert.Count(s, entries, "entries", test.wantCount)
			passert.Count(s, logs, "logs", test.wantCount)
			if len(test.wantRoot) > 0 {
				roots := beam.ParDo(s, func(e *batchmap.Entry) string { return fmt.Sprintf("%x", e.HashValue) }, entries)
				passert.Equals(s, roots, test.wantRoot)
			}
			if len(test.wantVersions) > 0 {
				versions := beam.ParDo(s, func(l *ModuleVersionLog) []string { return l.Versions }, logs)
				passert.Equals(s, versions, test.wantVersions)
			}
			err := ptest.Run(p)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
