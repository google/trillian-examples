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
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/client"
	"github.com/google/trillian-examples/formats/log"
)

func TestGetWitnessCheckpoint(t *testing.T) {
	for _, test := range []struct {
		desc    string
		body    []byte
		want    api.LogCheckpoint
		wantErr bool
	}{
		{
			desc: "valid 1",
			body: mustSignCPNote(t, "Firmware Transparency Log v0\n1\nEjQ=\n123\n"),
			want: api.LogCheckpoint{
				Checkpoint: log.Checkpoint{
					Ecosystem: "Firmware Transparency Log v0",
					Size:      1,
					Hash:      []byte{0x12, 0x34},
				},
				TimestampNanos: 123,
			},
		}, {
			desc: "valid 2",
			body: mustSignCPNote(t, "Firmware Transparency Log v0\n10\nNBI=\n1230\n"),
			want: api.LogCheckpoint{
				Checkpoint: log.Checkpoint{
					Ecosystem: "Firmware Transparency Log v0",
					Size:      10,
					Hash:      []byte{0x34, 0x12},
				},
				TimestampNanos: 1230,
			},
		}, {
			desc:    "garbage",
			body:    []byte("garbage"),
			wantErr: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, api.WitnessGetCheckpoint) {
					t.Fatalf("Got unexpected HTTP request on %q", r.URL.Path)
				}
				fmt.Fprint(w, string(test.body))
			}))
			defer ts.Close()

			tsURL, err := url.Parse((ts.URL))
			if err != nil {
				t.Fatalf("Failed to parse test server URL: %v", err)
			}
			wc := client.WitnessClient{
				URL:            tsURL,
				LogSigVerifier: mustGetCPVerifier(t),
			}
			cp, err := wc.GetWitnessCheckpoint()
			switch {
			case err != nil && !test.wantErr:
				t.Fatalf("Got unexpected error %q", err)
			case err == nil && test.wantErr:
				t.Fatal("Got no error, but wanted error")
			case err != nil && test.wantErr:
				// expected error
			default:
				// TODO(al): fix sigs here
				cp.Envelope = nil

				if d := cmp.Diff(*cp, test.want); len(d) != 0 {
					t.Fatalf("Got checkpoint with diff: %s", d)
				}
			}
		})
	}
}
