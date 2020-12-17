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

func TestMakeVersionList(t *testing.T) {
	tests := []struct {
		name     string
		metadata []Metadata

		wantCount int
		wantRoot  string
	}{
		{
			name: "single module single metadata",
			metadata: []Metadata{
				{
					Module:  "foo",
					Version: "1",
					ID:      1,
				},
			},
			wantCount: 1,
			wantRoot:  "18d27566bd1ac66b2332d8c54ad43f7bb22079c906d05f491f3f07a28d5c6990",
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
			wantCount: 1,
			wantRoot:  "fb687ea3931784e44d6c432af406a57e4ebc35cb53a8833569f6dfa086ebee93",
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
			wantCount: 1,
			wantRoot:  "fb687ea3931784e44d6c432af406a57e4ebc35cb53a8833569f6dfa086ebee93",
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

			entries := MakeVersionList(s, metadata)

			passert.Count(s, entries, "entries", test.wantCount)
			if len(test.wantRoot) > 0 {
				roots := beam.ParDo(s, func(e *batchmap.Entry) string { return fmt.Sprintf("%x", e.HashValue) }, entries)
				passert.Equals(s, roots, test.wantRoot)
			}
			err := ptest.Run(p)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
