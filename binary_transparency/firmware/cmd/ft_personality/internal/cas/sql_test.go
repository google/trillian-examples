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

package cas

import (
	"bytes"
	"crypto/sha512"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3" // Load drivers for sqlite3
)

func TestRoundTrip(t *testing.T) {
	for _, test := range []struct {
		desc      string
		image     []byte
		extraRuns int
	}{
		{
			desc:  "simple",
			image: []byte("simple"),
		}, {
			desc:      "check no failure on inserting same data twice",
			image:     []byte("simple"),
			extraRuns: 1,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			db, err := sql.Open("sqlite3", ":memory:")
			if err != nil {
				t.Error("failed to open temporary in-memory DB", err)
			}
			store, err := NewBinaryStorage(db)
			if err != nil {
				t.Error("failed to create CAS", err)
			}

			keyBs := sha512.Sum512(test.image)
			key, value := keyBs[:], test.image

			for i := 0; i < test.extraRuns+1; i++ {
				if err := store.Store(key, value); err != nil {
					t.Error("failed to store into CAS", err)
				}
				got, err := store.Retrieve(key)
				if err != nil {
					t.Error("failed to retrieve from CAS", err)
				}
				if !bytes.Equal(got, value) {
					t.Errorf("got != want (%x, %x)", got, value)
				}
			}
		})
	}
}
