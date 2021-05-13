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

package format

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestUnmarshalProof(t *testing.T) {
	for _, test := range []struct {
		desc    string
		m       string
		want    Proof
		wantErr bool
	}{
		{
			desc: "valid one",
			m:    "b25l\ndHdv\ndGhyZWU=\n",
			want: Proof{[]byte("one"), []byte("two"), []byte("three")},
		}, {
			desc: "valid two",
			m:    "Zm91cg==\nZml2ZQ==\nc2l4\nc2V2ZW4=\nZWlnaHQ=",
			want: Proof{[]byte("four"), []byte("five"), []byte("six"), []byte("seven"), []byte("eight")},
		}, {
			desc: "valid trailing newline",
			m:    "c2l4\nc2V2ZW4=\nZWlnaHQ=\n",
			want: Proof{[]byte("six"), []byte("seven"), []byte("eight")},
		}, {
			desc:    "invalid base64",
			m:       "c2l4=\nNOT-BASE64!\nZWlnaHQ=\n",
			wantErr: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			var got Proof
			if gotErr := got.Unmarshal([]byte(test.m)); (gotErr != nil) != test.wantErr {
				t.Fatalf("Unmarshal = %q, wantErr: %T", gotErr, test.wantErr)
			}
			if diff := cmp.Diff(test.want, got); len(diff) != 0 {
				t.Fatalf("Unmarshal = diff %s", diff)
			}
		})
	}
}
