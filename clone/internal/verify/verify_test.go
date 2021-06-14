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

package verify

import (
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/google/trillian/merkle/rfc6962/hasher"
	_ "github.com/mattn/go-sqlite3" // Load drivers for sqlite3
)

func TestRoot(t *testing.T) {
	leaves := make([][]byte, 64)
	for i := range leaves {
		leaves[i] = []byte(fmt.Sprintf("leaf %d", i))
	}
	sl := func(start, end uint64, out chan []byte, errc chan error) {
		defer close(out)
		stop := int(end)
		if stop > len(leaves) {
			stop = len(leaves)
		}
		for i := int(start); i < stop; i++ {
			out <- leaves[i]
		}
	}
	h := hasher.DefaultHasher
	lh := func(_ uint64, preimage []byte) []byte {
		return h.HashLeaf(preimage)
	}

	for _, test := range []struct {
		desc     string
		count    uint64
		wantRoot string
		wantErr  bool
	}{{
		desc:     "one leaf",
		count:    1,
		wantRoot: "G7l9zCFjXUfiZj79/QoXRobZjdcBNS3SzQbotD/T0wU",
	}, {
		desc:     "17 leaves",
		count:    17,
		wantRoot: "Ru8bykxkgM1l5Q4pzBw3XbNnEc1QJF7NPmsxDG4qOD8",
	}, {
		desc:     "all leaves",
		count:    64,
		wantRoot: "8mHiNpLZeP2sP9lJ21SVlApDeuZxuabd6aphGNADZS8",
	}, {
		desc:    "too many leaves",
		count:   65,
		wantErr: true,
	},
	} {
		t.Run(test.desc, func(t *testing.T) {
			v := NewLogVerifier(sl, lh, h.HashChildren)
			got, err := v.MerkleRoot(test.count)
			if gotErr := err != nil; test.wantErr != gotErr {
				t.Errorf("expected err (%t) but got: %q", test.wantErr, err)
			}
			if !test.wantErr {
				gotb64 := base64.RawStdEncoding.EncodeToString(got)
				if gotb64 != test.wantRoot {
					t.Errorf("got %q but wanted root %q", gotb64, test.wantRoot)
				}
			}
		})
	}
}
