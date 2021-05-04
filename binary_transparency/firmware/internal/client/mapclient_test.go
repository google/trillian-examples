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

package client_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/client"
)

func TestMapCheckpoint(t *testing.T) {
	for _, test := range []struct {
		desc    string
		body    string
		want    api.MapCheckpoint
		wantErr bool
	}{
		{
			desc: "valid 1",
			body: `{ "Revision": 42, "LogSize": 1, "LogCheckpoint": "QyE=", "RootHash": "EjQ="}`,
			want: api.MapCheckpoint{Revision: 42, LogSize: 1, LogCheckpoint: []byte{0x43, 0x21}, RootHash: []byte{0x12, 0x34}},
		}, {
			desc: "valid 2",
			body: `{ "Revision": 42, "LogSize": 10, "LogCheckpoint": "/u1C", "RootHash": "NBI="}`,
			want: api.MapCheckpoint{Revision: 42, LogSize: 10, LogCheckpoint: []byte{0xfe, 0xed, 0x42}, RootHash: []byte{0x34, 0x12}},
		}, {
			desc:    "garbage",
			body:    `garbage`,
			wantErr: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, api.MapHTTPGetCheckpoint) {
					t.Fatalf("Got unexpected HTTP request on %q", r.URL.Path)
				}
				fmt.Fprintln(w, test.body)
			}))
			defer ts.Close()

			c, err := client.NewMapClient(ts.URL)
			if err != nil {
				t.Fatalf("Failed to create client: %q", err)
			}
			cp, err := c.MapCheckpoint()
			switch {
			case err != nil && !test.wantErr:
				t.Fatalf("Got unexpected error %q", err)
			case err == nil && test.wantErr:
				t.Fatal("Got no error, but wanted error")
			case err != nil && test.wantErr:
				// expected error
			default:
				if d := cmp.Diff(cp, test.want); len(d) != 0 {
					t.Fatalf("Got checkpoint with diff: %s", d)
				}
			}
		})
	}
}

func TestAggregation(t *testing.T) {
	for _, test := range []struct {
		desc       string
		rev, index uint64
		bodies     map[string]string
		wantAgg    []byte
		wantErr    bool
	}{
		{
			desc:  "valid 1",
			rev:   1,
			index: 0,
			bodies: map[string]string{
				"/ftmap/v0/tile/in-revision/1/at-path/":                       `{"Path":"","Leaves":[{"Path":"Rg==","Hash":"M7DmUN5R2auo88WMjg+EcijUzfX085QdHuTzx7Rrwgs="},{"Path":"7A==","Hash":"fK4jTvvbd90D29jYrsBrmWNG83416K1WhgS5T5qYpEI="}]}`,
				"/ftmap/v0/tile/in-revision/1/at-path/Rg==":                   `{"Path":"Rg==","Leaves":[{"Path":"IRCmyCYwSorPfJ/NXqlkZdVYvs4KKtRYuV27zLJjCg==","Hash":"sE0GFv87tf0j7YciRBVFk4pFKExwVRhwykxdPz70Dxw="}]}`,
				"/ftmap/v0/aggregation/in-revision/1/for-firmware-at-index/0": `{"Index":0,"Good":true}`,
			},
			wantAgg: []byte(`{"Index":0,"Good":true}`),
		}, {
			desc:  "valid 2",
			rev:   2,
			index: 1,
			bodies: map[string]string{
				"/ftmap/v0/tile/in-revision/2/at-path/":                       `{"Path":"","Leaves":[{"Path":"Rg==","Hash":"M7DmUN5R2auo88WMjg+EcijUzfX085QdHuTzx7Rrwgs="},{"Path":"7A==","Hash":"fK4jTvvbd90D29jYrsBrmWNG83416K1WhgS5T5qYpEI="}]}`,
				"/ftmap/v0/tile/in-revision/2/at-path/7A==":                   `{"Path":"7A==","Leaves":[{"Path":"Kk19hKuMqnXxlJGG0LQ5y8LbzyPhmCCDMxRmPAowFw==","Hash":"+d6n+Cubqrkvx6vwQg0f2M3ZPub3a8jf/HICam0T3sM="},{"Path":"z+HuPYEme3qpfllqffSoL8jKc8VLtf3njh/nVoksCA==","Hash":"rxQDwfN/PhVD+lF2FtVkzUb9ha1G+4OHE7ZaIvSow9Y="}]}`,
				"/ftmap/v0/aggregation/in-revision/2/for-firmware-at-index/1": `{"Index":1,"Good":false}`,
			},
			wantAgg: []byte(`{"Index":1,"Good":false}`),
		}, {
			desc:  "garbage",
			rev:   42,
			index: 22,
			bodies: map[string]string{
				"/ftmap/v0/tile/in-revision/42/at-path/":                        `moose`,
				"/ftmap/v0/tile/in-revision/42/at-path/RA==":                    `loose`,
				"/ftmap/v0/aggregation/in-revision/42/for-firmware-at-index/22": `house`,
			},
			wantErr: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if body, ok := test.bodies[r.URL.Path]; ok {
					fmt.Fprint(w, body)
				} else {
					t.Fatalf("Got unexpected HTTP request on %q", r.URL.Path)
				}
			}))
			defer ts.Close()

			c, err := client.NewMapClient(ts.URL)
			if err != nil {
				t.Fatalf("Failed to create client: %q", err)
			}
			agg, proof, err := c.Aggregation(context.Background(), test.rev, test.index)
			switch {
			case err != nil && !test.wantErr:
				t.Fatalf("Got unexpected error %q", err)
			case err == nil && test.wantErr:
				t.Fatal("Got no error, but wanted error")
			case err != nil && test.wantErr:
				// expected error
			default:
				if !bytes.Equal(agg, test.wantAgg) {
					t.Errorf("Got wrong aggregation (%x != %x)", agg, test.wantAgg)
				}
				// TODO(mhutchinson): more fully test the generated inclusion proof.
				if proof.Key == nil || proof.Value == nil || len(proof.Proof) != 256 {
					t.Errorf("Generated proof is malformed: %+v", proof)
				}
			}
		})
	}
}
