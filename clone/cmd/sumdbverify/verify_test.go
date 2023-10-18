// Copyright 2023 Google LLC. All Rights Reserved.
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

package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/google/trillian-examples/clone/logdb"
)

// golang.org/x/text v0.3.0 h1:g61tztE5qeGQ89tm6NTjjM9VPIm088od1l6aSorWRWg=
// golang.org/x/text v0.3.0/go.mod h1:NqM8EUOU14njkJ3fqMW+pc6Ldnwhi/IjpwHt7yyuwOQ=
const leafFormat = "%s %s h1:%s\n%s %s/go.mod h1:%s\n"

func TestVerifyLeaves(t *testing.T) {
	testCases := []struct {
		desc    string
		leaves  [][]byte
		wantErr bool
	}{
		{
			desc: "good",
			leaves: [][]byte{
				makeLeaf("apple", "v1.0.0", "salt"),
				makeLeaf("ban/a/na", "v1.1.0", "salt"),
			},
		},
		{
			desc: "duplicate with same hash",
			leaves: [][]byte{
				makeLeaf("a", "v1.0.0", "salt"),
				makeLeaf("b", "v1.1.0", "salt"),
				makeLeaf("a", "v1.0.0", "salt"),
			},
		},
		{
			desc: "duplicate with different hash",
			leaves: [][]byte{
				makeLeaf("a", "v1.0.0", "salt"),
				makeLeaf("b", "v1.1.0", "salt"),
				makeLeaf("a", "v1.0.0", "spice"),
			},
			wantErr: true,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			source := leafSource{leaves: tC.leaves}

			size, err := verifyLeaves(context.Background(), source)
			switch {
			case err != nil && !tC.wantErr:
				t.Fatalf("Got unexpected error %q", err)
			case err == nil && tC.wantErr:
				t.Fatal("Got no error, but wanted error")
			case err != nil && tC.wantErr:
				// expected error
			default:
				if got, want := size, uint64(len(tC.leaves)); got != want {
					t.Errorf("expected %d but got %d", want, got)
				}
			}
		})
	}
}

func makeLeaf(mod, ver, salt string) []byte {
	// repoHash and modHash are deterministically derived from {mod, version, salt} such
	// that they are different from each other, but unique for any given input tuple.
	repoHash := sha256.Sum256([]byte(mod + ver + salt))
	repoHashString := base64.StdEncoding.EncodeToString(repoHash[:])

	modHash := sha256.Sum256([]byte(ver + mod + salt))
	modHashString := base64.StdEncoding.EncodeToString(modHash[:])

	return []byte(fmt.Sprintf(leafFormat, mod, ver, repoHashString, mod, ver, modHashString))
}

type leafSource struct {
	leaves [][]byte
}

func (s leafSource) GetLatestCheckpoint(ctx context.Context) (size uint64, checkpoint []byte, compactRange [][]byte, err error) {
	return uint64(len(s.leaves)), nil, nil, nil
}

func (s leafSource) StreamLeaves(ctx context.Context, start, end uint64, out chan<- logdb.StreamResult) {
	for _, v := range s.leaves {
		out <- logdb.StreamResult{
			Leaf: v,
		}
	}
	close(out)
}
