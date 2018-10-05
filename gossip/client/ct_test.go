// Copyright 2018 Google Inc. All Rights Reserved.
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
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/certificate-transparency-go/jsonclient"
	"github.com/google/certificate-transparency-go/tls"
	"github.com/google/trillian-examples/gossip/api"
	"github.com/google/trillian-examples/gossip/client"

	ct "github.com/google/certificate-transparency-go"
)

func TestAddCTSTH(t *testing.T) {
	ctx := context.Background()

	th := ct.TreeHeadSignature{
		Version:       ct.V1,
		SignatureType: ct.TreeHashSignatureType,
		TreeSize:      100,
		Timestamp:     0x1000000,
	}
	rand.Read(th.SHA256RootHash[:])
	thData, err := tls.Marshal(th)
	if err != nil {
		t.Fatalf("Failed to create test data: %v", err)
	}

	entry := api.TimestampedEntry{
		SourceID:        []byte("test"),
		BlobData:        thData,
		SourceSignature: []byte{0xDD},
		HubTimestamp:    0x0000000012345678,
	}
	data, err := tls.Marshal(entry)
	if err != nil {
		t.Fatalf("Failed to create test data: %v", err)
	}
	signer, pubKeyDER := generateKeys(t)

	tests := []struct {
		desc    string
		sth     *ct.SignedTreeHead
		body    string
		wantErr string
	}{
		{
			desc: "Valid STH",
			sth: &ct.SignedTreeHead{
				Version:        ct.V1,
				TreeSize:       th.TreeSize,
				Timestamp:      th.Timestamp,
				SHA256RootHash: th.SHA256RootHash,
				TreeHeadSignature: ct.DigitallySigned{
					Algorithm: tls.SignatureAndHashAlgorithm{
						Hash:      tls.SHA256,
						Signature: tls.ECDSA,
					},
					Signature: []byte{0xDD},
				},
			},
			body: fmt.Sprintf(`{"timestamped_entry":"%s", "hub_signature":"%s"}`, signData(t, signer, data)...),
		},

		{
			desc: "Malformed STH",
			sth: &ct.SignedTreeHead{
				Version:        ct.V1 + 100,
				TreeSize:       th.TreeSize,
				Timestamp:      th.Timestamp,
				SHA256RootHash: th.SHA256RootHash,
			},
			wantErr: "unsupported STH version",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ts := serveAt(t, api.AddSignedBlobPath, http.StatusOK, test.body)
			defer ts.Close()
			cl, err := client.New(ts.URL, &http.Client{}, jsonclient.Options{PublicKeyDER: pubKeyDER})
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			got, gotErr := cl.AddCTSTH(ctx, "test", test.sth)
			if gotErr != nil {
				if len(test.wantErr) == 0 {
					t.Errorf("AddCTSTH()=nil,%v; want _, nil", gotErr)
				} else if !strings.Contains(gotErr.Error(), test.wantErr) {
					t.Errorf("AddCTSTH()=nil,%v; want _, err containing %q", gotErr, test.wantErr)
				}
				return
			}
			if len(test.wantErr) != 0 {
				t.Errorf("AddCTSTH()=%v,nil; want _, err containing %q", got, test.wantErr)
			}
			if got == nil {
				t.Errorf("AddCTSTH()=nil; want non-nil")
			}
		})
	}
}
