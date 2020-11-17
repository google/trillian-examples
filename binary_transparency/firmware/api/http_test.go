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

package api_test

import (
	"testing"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
)

func TestLogCheckpointString(t *testing.T) {
	for _, test := range []struct {
		cp   api.LogCheckpoint
		want string
	}{
		{
			cp:   api.LogCheckpoint{},
			want: "{size 0 @ 0 root: 0x}",
		}, {
			cp:   api.LogCheckpoint{TreeSize: 10, TimestampNanos: 234, RootHash: []byte{0x12, 0x34, 0x56}},
			want: "{size 10 @ 234 root: 0x123456}",
		},
	} {
		if got, want := test.cp.String(), test.want; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}
