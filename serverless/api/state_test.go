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

// package api_test contains tests for the api package.
package api_test

import (
	"fmt"
	"testing"

	"github.com/google/trillian-examples/serverless/api"
)

func TestNodeKey(t *testing.T) {
	for _, test := range []struct {
		level uint
		index uint64
		want  uint
	}{
		{
			level: 0,
			index: 0,
			want:  0,
		}, {
			level: 1,
			index: 0,
			want:  1,
		}, {
			level: 1,
			index: 1,
			want:  5,
		},
	} {
		t.Run(fmt.Sprintf("level %d, index %d", test.level, test.index), func(t *testing.T) {
			if got, want := api.TileNodeKey(test.level, test.index), test.want; got != want {
				t.Fatalf("got %d want %d", got, want)
			}
		})
	}
}
