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

package main

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCertLeafFetcher(t *testing.T) {
	for _, test := range []struct {
		name    string
		start   uint64
		count   uint
		urls    map[string]string
		want    []string
		wantErr bool
	}{
		{
			name:  "first leaf",
			start: 0,
			count: 1,
			urls:  map[string]string{"ct/v1/get-entries?start=0&end=0": `{"entries":[{"leaf_input": "dGhpcyBjb3VsZCBiZSBhIGNlcnQ="}]}`},
			want:  []string{"this could be a cert"},
		},
		{
			name:  "three leaves",
			start: 101,
			count: 3,
			urls:  map[string]string{"ct/v1/get-entries?start=101&end=103": `{"entries":[{"leaf_input": "b25lIG9oIG9uZQ=="},{"leaf_input": "b25lIG9oIHR3bw=="},{"leaf_input": "b25lIG9oIHRocmVl"}]}`},
			want:  []string{"one oh one", "one oh two", "one oh three"},
		},
		{
			name:    "too many returned",
			start:   42,
			count:   1,
			urls:    map[string]string{"ct/v1/get-entries?start=42&end=42": `{"entries":[{"leaf_input": "b25l"},{"leaf_input": "dHdv"}]}`},
			wantErr: true,
		},
		{
			name:    "not enough returned",
			start:   42,
			count:   2,
			urls:    map[string]string{"ct/v1/get-entries?start=42&end=43": `{"entries":[{"leaf_input": "b25l"}]}`},
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			clf := ctFetcher{&fakeFetcher{test.urls}}
			leaves := make([][]byte, test.count)
			_, err := clf.Batch(test.start, leaves)
			switch {
			case err != nil && !test.wantErr:
				t.Fatalf("Got unexpected error %q", err)
			case err == nil && test.wantErr:
				t.Fatal("Got no error, but wanted error")
			case err != nil && test.wantErr:
				// expected error
			default:
				gotStrings := make([]string, len(leaves))
				for i, bs := range leaves {
					gotStrings[i] = string(bs)
				}
				if d := cmp.Diff(gotStrings, test.want); len(d) != 0 {
					t.Fatalf("Got different result: %s", d)
				}
			}
		})
	}
}

type fakeFetcher struct {
	values map[string]string
}

func (f *fakeFetcher) GetData(path string) ([]byte, error) {
	res, ok := f.values[path]
	if !ok {
		return nil, fmt.Errorf("could not find '%s'", path)
	}
	return []byte(res), nil
}
