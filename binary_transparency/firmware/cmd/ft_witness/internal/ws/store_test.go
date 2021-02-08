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

package ws

import (
	"testing"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
)

const (
	dbFp = "/tmp/wdb.db"
)

func TestRoundTrip(t *testing.T) {
	for _, test := range []struct {
		desc string
		cp   api.LogCheckpoint
	}{
		{
			desc: "initial checkpoint test",
			cp:   api.LogCheckpoint{TreeSize: 1, RootHash: []byte("Standard Root Hash"), TimestampNanos: 41672},
		}, {
			desc: "check over-write of checkpoint",
			cp:   api.LogCheckpoint{TreeSize: 2, RootHash: []byte("New  Root Hash"), TimestampNanos: 91982},
		},
	} {
		t.Run(test.desc, func(t *testing.T) {

			store, err := NewStorage(dbFp)
			if err != nil {
				t.Error("failed to create storage", err)
			}
			want := test.cp
			if err := store.StoreCP(test.cp); err != nil {
				t.Error("failed to store into Witness Store", err)
			}
			got, err := store.RetrieveCP()
			if err != nil {
				t.Error("failed to retrieve from Witness Store", err)
			}
			if got.TimestampNanos != want.TimestampNanos {
				t.Errorf("got '%s' want '%s'", got, want)
			}

		})
	}
}
