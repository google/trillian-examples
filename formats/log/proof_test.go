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

package log_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian-examples/formats/log"
)

func TestMarshalProof(t *testing.T) {
	for _, test := range []struct {
		desc string
		p    log.Proof
		want string
	}{
		{
			desc: "valid",
			p: log.Proof{
				[]byte("one"), []byte("two"), []byte("three"),
			},
			want: "b25l\ndHdv\ndGhyZWU=\n",
		}, {
			desc: "valid empty",
			p:    log.Proof{},
			want: "",
		}, {
			desc: "valid default entry",
			p: log.Proof{
				[]byte("one"), []byte{}, []byte("three"),
			},
			want: "b25l\n\ndGhyZWU=\n",
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			got := test.p.Marshal()
			if got != test.want {
				t.Fatalf("Got %q, want %q", got, test.want)
			}
		})
	}
}

func TestUnmarshalProof(t *testing.T) {
	for _, test := range []struct {
		desc    string
		m       string
		want    log.Proof
		wantErr bool
	}{
		{
			desc: "valid one",
			m:    "b25l\ndHdv\ndGhyZWU=\n",
			want: log.Proof{[]byte("one"), []byte("two"), []byte("three")},
		}, {
			desc: "valid two",
			m:    "Zm91cg==\nZml2ZQ==\nc2l4\nc2V2ZW4=\nZWlnaHQ=\n",
			want: log.Proof{[]byte("four"), []byte("five"), []byte("six"), []byte("seven"), []byte("eight")},
		}, {
			desc:    "invalid - missing newline after last hash",
			m:       "c2l4\nc2V2ZW4=\nZWlnaHQ=",
			wantErr: true,
		}, {
			desc:    "invalid base64",
			m:       "c2l4=\nNOT-BASE64!\nZWlnaHQ=\n",
			wantErr: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			var got log.Proof
			if gotErr := got.Unmarshal([]byte(test.m)); (gotErr != nil) != test.wantErr {
				t.Fatalf("Unmarshal = %q, wantErr: %T", gotErr, test.wantErr)
			}
			if diff := cmp.Diff(test.want, got); len(diff) != 0 {
				t.Fatalf("Unmarshal = diff %s", diff)
			}
		})
	}
}
