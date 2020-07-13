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
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/certificate-transparency-go/jsonclient"
	"github.com/google/certificate-transparency-go/x509"
	"github.com/google/trillian-examples/gossip/api"
	"github.com/google/trillian-examples/gossip/client"
	"github.com/google/trillian/testonly"

	tcrypto "github.com/google/trillian/crypto"
)

// serveAt returns a test HTTP server that returns a canned response status/body for a given path.
func serveAt(t *testing.T, path string, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != api.PathPrefix+path {
			t.Fatalf("Incorrect URL path: %s", r.URL.Path)
		}
		if status != 0 {
			w.WriteHeader(status)
		}
		fmt.Fprint(w, body)
	}))
}

// generateKeys creates a new ECDSA private key and returns it as a tcrypto.Signer,
// together with the DER for the associated public key in PKIX format.
func generateKeys(t *testing.T) (*tcrypto.Signer, []byte) {
	t.Helper()
	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate ECDSA key: %v", err)
	}
	pubKeyDER, err := x509.MarshalPKIXPublicKey(ecdsaKey.Public())
	if err != nil {
		t.Fatalf("Failed to marshal public key to DER: %v", err)
	}
	signer := tcrypto.NewSigner(6962, ecdsaKey, crypto.SHA256)
	return signer, pubKeyDER
}

// signData returns data, signature-over-data as a 2 element slice of base-64 encoded strings.
// Returned as a slice of interface{} to allow both results to be passed to Formatf().
func signData(t *testing.T, signer *tcrypto.Signer, data []byte) []interface{} {
	sig, err := signer.Sign(data)
	if err != nil {
		t.Fatalf("Failed to sign TLS-encoded data: %v", err)
	}
	return []interface{}{b64(data), b64(sig)}
}

var (
	h2b = testonly.MustHexDecode
	b64 = base64.StdEncoding.EncodeToString
)

func TestAddSignedData(t *testing.T) {
	ctx := context.Background()
	entry := api.TimestampedEntry{
		SourceID:        []byte("https://a-log.com"),
		BlobData:        []byte{0x01},
		SourceSignature: []byte{0x02},
		HubTimestamp:    0x0000000012345678,
	}
	data := h2b(
		"0011" + "68747470733a2f2f612d6c6f672e636f6d" +
			"0001" + "01" +
			"0001" + "02" +
			"0000000012345678")
	signer, pubKeyDER := generateKeys(t)

	tests := []struct {
		desc    string
		body    string
		status  int
		wantErr string
	}{
		{
			desc: "Valid SGT",
			body: fmt.Sprintf(`{"timestamped_entry":"%s", "hub_signature":"%s"}`, signData(t, signer, data)...),
		},
		{
			desc:    "Malformed JSON",
			body:    "fllumpf",
			wantErr: "deadline exceeded", // due to repeated retries
		},
		{
			desc:    "Malformed SGT",
			body:    fmt.Sprintf(`{"timestamped_entry":"%s", "hub_signature":"%s"}`, signData(t, signer, []byte{0x00})...),
			wantErr: "truncated variable-length integer",
		},
		{
			desc:    "SGT Trailing Data",
			body:    fmt.Sprintf(`{"timestamped_entry":"%s", "hub_signature":"%s"}`, signData(t, signer, append(data, 0xff))...),
			wantErr: "trailing data",
		},
		{
			desc:    "Invalid SGT signature",
			body:    fmt.Sprintf(`{"timestamped_entry":"%s", "hub_signature":"AAAA"}`, b64(data)),
			wantErr: "signature verification failed",
		},
		{
			desc: "SGT source ID mismatch",
			body: fmt.Sprintf(`{"timestamped_entry":"%s", "hub_signature":"%s"}`,
				signData(t, signer, h2b("0011"+"68747470733a2f2f612d6c6f672e636f6c" /* was ...6d */ +"0001"+"01"+"0001"+"02"+"0000000012345678"))...),
			wantErr: "source ID",
		},
		{
			desc: "SGT data mismatch",
			body: fmt.Sprintf(`{"timestamped_entry":"%s", "hub_signature":"%s"}`,
				signData(t, signer, h2b("0011"+"68747470733a2f2f612d6c6f672e636f6d"+"0001"+"02" /* was 01 */ +"0001"+"02"+"0000000012345678"))...),
			wantErr: "head data",
		},
		{
			desc:    "POST failure",
			body:    "Utter failure",
			status:  404,
			wantErr: "404 Not Found",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ts := serveAt(t, api.AddSignedBlobPath, test.status, test.body)
			defer ts.Close()
			cl, err := client.New(ts.URL, &http.Client{}, jsonclient.Options{PublicKeyDER: pubKeyDER})
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
			defer cancel()

			got, gotErr := cl.AddSignedBlob(ctx, string(entry.SourceID), entry.BlobData, entry.SourceSignature)
			if gotErr != nil {
				if len(test.wantErr) == 0 {
					t.Errorf("AddSignedBlob()=nil,%v; want _, nil", gotErr)
				} else if !strings.Contains(gotErr.Error(), test.wantErr) {
					t.Errorf("AddSignedBlob()=nil,%v; want _, err containing %q", gotErr, test.wantErr)
				}
				return
			}
			if len(test.wantErr) != 0 {
				t.Errorf("AddSignedBlob()=%v,nil; want _, err containing %q", got, test.wantErr)
			}
			if got == nil {
				t.Errorf("AddSignedBlob()=nil; want non-nil")
			}
		})
	}
}

func TestGetSTH(t *testing.T) {
	ctx := context.Background()
	treeHead := api.HubTreeHead{
		TreeSize:  0x100,
		Timestamp: 0x123456,
		RootHash:  []byte{0x01, 0x02, 0x03, 0x04},
	}
	data := h2b("0000000000000100" + "0000000000123456" + "0004" + "01020304")
	signer, pubKeyDER := generateKeys(t)

	tests := []struct {
		desc    string
		body    string
		status  int
		wantErr string
	}{
		{
			desc: "Valid STH",
			body: fmt.Sprintf(`{"head_data":"%s", "hub_signature":"%s"}`, signData(t, signer, data)...),
		},
		{
			desc:    "Malformed JSON",
			body:    "fllumpf",
			wantErr: "invalid character",
		},
		{
			desc:    "Malformed STH",
			body:    fmt.Sprintf(`{"head_data":"%s", "hub_signature":"%s"}`, signData(t, signer, []byte{0x00})...),
			wantErr: "truncated",
		},
		{
			desc:    "STH Trailing Data",
			body:    fmt.Sprintf(`{"head_data":"%s", "hub_signature":"%s"}`, signData(t, signer, append(data, 0xff))...),
			wantErr: "trailing data",
		},
		{
			desc:    "Invalid STH signature",
			body:    fmt.Sprintf(`{"head_data":"%s", "hub_signature":"AAAA"}`, b64(data)),
			wantErr: "signature verification failed",
		},
		{
			desc:    "GET failure",
			body:    "Utter failure",
			status:  404,
			wantErr: "404 Not Found",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ts := serveAt(t, api.GetSTHPath, test.status, test.body)
			defer ts.Close()
			cl, err := client.New(ts.URL, &http.Client{}, jsonclient.Options{PublicKeyDER: pubKeyDER})
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			got, gotErr := cl.GetSTH(ctx)
			if gotErr != nil {
				if len(test.wantErr) == 0 {
					t.Errorf("GetSTH()=nil,%v; want _, nil", gotErr)
				} else if !strings.Contains(gotErr.Error(), test.wantErr) {
					t.Errorf("GetSTH()=nil,%v; want _, err containing %q", gotErr, test.wantErr)
				}
				return
			}
			if len(test.wantErr) != 0 {
				t.Errorf("GetSTH()=%v,nil; want nil, err containing %q", got, test.wantErr)
			}
			want := &api.SignedHubTreeHead{
				TreeHead:     treeHead,
				HubSignature: got.HubSignature, // not deterministic so copy over
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("GetSTH()=%+v; want %+v", got, want)
			}
		})
	}
}

func TestGetSTHConsistency(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		desc          string
		first, second uint64
		body          string
		status        int
		wantErr       string
	}{
		{
			desc:   "Valid",
			first:  1,
			second: 10,
			body:   fmt.Sprintf(`{"consistency":["%s"]}`, b64([]byte{0x01, 0x02})),
		},
		{
			desc:    "Inverted",
			first:   10,
			second:  1,
			wantErr: "range inverted",
		},
		{
			desc:    "Malformed JSON",
			first:   1,
			second:  10,
			body:    "fllumpf",
			wantErr: "invalid character",
		},
		{
			desc:    "Malformed base-64",
			first:   1,
			second:  10,
			body:    `{"consistency":"@@"}`,
			wantErr: "cannot unmarshal",
		},
		{
			desc:    "GET failure",
			first:   1,
			second:  10,
			body:    "Utter failure",
			status:  404,
			wantErr: "404 Not Found",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ts := serveAt(t, api.GetSTHConsistencyPath, test.status, test.body)
			defer ts.Close()
			cl, err := client.New(ts.URL, &http.Client{}, jsonclient.Options{})
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			got, gotErr := cl.GetSTHConsistency(ctx, test.first, test.second)
			if gotErr != nil {
				if len(test.wantErr) == 0 {
					t.Errorf("GetSTHConsistency(%d, %d)=nil,%v; want _, nil", test.first, test.second, gotErr)
				} else if !strings.Contains(gotErr.Error(), test.wantErr) {
					t.Errorf("GetSTHConsistency(%d, %d)=nil,%v; want _, err containing %q", test.first, test.second, gotErr, test.wantErr)
				}
				return
			}
			if len(test.wantErr) != 0 {
				t.Errorf("GetSTHConsistency(%d, %d)=%v,nil; want nil, err containing %q", test.first, test.second, got, test.wantErr)
			}
			want := [][]byte{{0x01, 0x02}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("GetSTHConsistency(%d, %d)=%+v; want %+v", test.first, test.second, got, want)
			}
		})
	}
}

func TestGetProofByHash(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		desc    string
		body    string
		status  int
		wantErr string
	}{
		{
			desc: "Valid",
			body: fmt.Sprintf(`{"leaf_index":2, "audit_path":["%s"]}`, b64([]byte{0x01, 0x02})),
		},
		{
			desc:    "Malformed JSON",
			body:    "fllumpf",
			wantErr: "invalid character",
		},
		{
			desc:    "Malformed base-64",
			body:    `{"leaf_index":4,"audit_path":"@@"}`,
			wantErr: "cannot unmarshal",
		},
		{
			desc:    "Malformed index",
			body:    fmt.Sprintf(`{"leaf_index":"AAAA","audit_path":"%s"}`, b64([]byte{0x01, 0x02})),
			wantErr: "cannot unmarshal",
		},
		{
			desc:    "GET failure",
			body:    "Utter failure",
			status:  404,
			wantErr: "404 Not Found",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ts := serveAt(t, api.GetProofByHashPath, test.status, test.body)
			defer ts.Close()
			cl, err := client.New(ts.URL, &http.Client{}, jsonclient.Options{})
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			got, gotErr := cl.GetProofByHash(ctx, []byte{0x11, 0x12}, 100)
			if gotErr != nil {
				if len(test.wantErr) == 0 {
					t.Errorf("GetProofByHash()=nil,%v; want _, nil", gotErr)
				} else if !strings.Contains(gotErr.Error(), test.wantErr) {
					t.Errorf("GetProofByHash()=nil,%v; want _, err containing %q", gotErr, test.wantErr)
				}
				return
			}
			if len(test.wantErr) != 0 {
				t.Errorf("GetProofByHash()=%v,nil; want nil, err containing %q", got, test.wantErr)
			}
			want := &api.GetProofByHashResponse{
				LeafIndex: 2,
				AuditPath: [][]byte{{0x01, 0x02}},
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("GetProofByHash()=%+v; want %+v", got, want)
			}
		})
	}
}

func TestGetRawEntries(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		desc       string
		start, end int64
		body       string
		status     int
		wantErr    string
	}{
		{
			desc:  "Valid",
			start: 1,
			end:   3,
			body:  fmt.Sprintf(`{"entries":["%s"]}`, b64([]byte{0x01, 0x02})),
		},
		{
			desc:    "Invalid parameter inverted",
			start:   3,
			end:     1,
			wantErr: "start should be <= end",
		},
		{
			desc:    "Invalid parameter negative",
			start:   -1,
			end:     1,
			wantErr: "start should be >= 0",
		},
		{
			desc:    "Malformed JSON",
			start:   1,
			end:     3,
			body:    "fllumpf",
			wantErr: "invalid character",
		},
		{
			desc:    "GET failure",
			start:   1,
			end:     3,
			body:    "Utter failure",
			status:  404,
			wantErr: "404 Not Found",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ts := serveAt(t, api.GetEntriesPath, test.status, test.body)
			defer ts.Close()
			cl, err := client.New(ts.URL, &http.Client{}, jsonclient.Options{})
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			got, gotErr := cl.GetRawEntries(ctx, test.start, test.end)
			if gotErr != nil {
				if len(test.wantErr) == 0 {
					t.Errorf("GetRawEntries(%d, %d)=nil,%v; want _, nil", test.start, test.end, gotErr)
				} else if !strings.Contains(gotErr.Error(), test.wantErr) {
					t.Errorf("GetRawEntries(%d, %d)=nil,%v; want _, err containing %q", test.start, test.end, gotErr, test.wantErr)
				}
				return
			}
			if len(test.wantErr) != 0 {
				t.Errorf("GetRawEntries(%d, %d)=%v,nil; want nil, err containing %q", test.start, test.end, got, test.wantErr)
			}
			want := [][]byte{{0x01, 0x02}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("GetRawEntries(%d, %d)=%+v; want %+v", test.start, test.end, got, want)
			}
		})
	}
}

func TestGetEntries(t *testing.T) {
	ctx := context.Background()
	entry := api.TimestampedEntry{
		SourceID:        []byte("https://a-log.com"),
		BlobData:        []byte{0x01},
		SourceSignature: []byte{0x02},
		HubTimestamp:    0x0000000012345678,
	}
	data := h2b(
		"0011" + "68747470733a2f2f612d6c6f672e636f6d" +
			"0001" + "01" +
			"0001" + "02" +
			"0000000012345678")

	tests := []struct {
		desc       string
		start, end int64
		body       string
		status     int
		wantErr    string
	}{
		{
			desc:  "Valid",
			start: 1,
			end:   3,
			body:  fmt.Sprintf(`{"entries":["%s"]}`, b64(data)),
		},
		{
			desc:    "Invalid trailing data",
			start:   1,
			end:     3,
			body:    fmt.Sprintf(`{"entries":["%s"]}`, b64(append(data, 0xff))),
			wantErr: "trailing data",
		},
		{
			desc:    "Invalid TLS data",
			start:   1,
			end:     3,
			body:    fmt.Sprintf(`{"entries":["%s"]}`, b64([]byte{0x01, 0x02})),
			wantErr: "tls: syntax error",
		},
		{
			desc:    "Invalid parameter inverted",
			start:   3,
			end:     1,
			wantErr: "start should be <= end",
		},
		{
			desc:    "Invalid parameter negative",
			start:   -1,
			end:     1,
			wantErr: "start should be >= 0",
		},
		{
			desc:    "Malformed JSON",
			start:   1,
			end:     3,
			body:    "fllumpf",
			wantErr: "invalid character",
		},
		{
			desc:    "GET failure",
			start:   1,
			end:     3,
			body:    "Utter failure",
			status:  404,
			wantErr: "404 Not Found",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ts := serveAt(t, api.GetEntriesPath, test.status, test.body)
			defer ts.Close()
			cl, err := client.New(ts.URL, &http.Client{}, jsonclient.Options{})
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			got, gotErr := cl.GetEntries(ctx, test.start, test.end)
			if gotErr != nil {
				if len(test.wantErr) == 0 {
					t.Errorf("GetEntries(%d, %d)=nil,%v; want _, nil", test.start, test.end, gotErr)
				} else if !strings.Contains(gotErr.Error(), test.wantErr) {
					t.Errorf("GetEntries(%d, %d)=nil,%v; want _, err containing %q", test.start, test.end, gotErr, test.wantErr)
				}
				return
			}
			if len(test.wantErr) != 0 {
				t.Errorf("GetEntries(%d, %d)=%v,nil; want nil, err containing %q", test.start, test.end, got, test.wantErr)
			}
			want := []*api.TimestampedEntry{&entry}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("GetEntries(%d, %d)=%+v; want %+v", test.start, test.end, got, want)
			}
		})
	}
}

func TestGetSourceKeys(t *testing.T) {
	ctx := context.Background()
	_, pubKeyDER := generateKeys(t)

	tests := []struct {
		desc     string
		body     string
		status   int
		wantErr  string
		wantKind string
	}{
		{
			desc:     "Valid",
			body:     fmt.Sprintf(`{"entries":[{"id":"https://source.example.com","pub_key":"%s"}]}`, b64(pubKeyDER)),
			wantKind: "",
		},
		{
			desc:     "Valid CT Source",
			body:     fmt.Sprintf(`{"entries":[{"id":"https://source.example.com","pub_key":"%s","kind":"RFC6962STH"}]}`, b64(pubKeyDER)),
			wantKind: "RFC6962STH",
		},
		{
			desc:     "Valid Trillian SLR Source",
			body:     fmt.Sprintf(`{"entries":[{"id":"https://source.example.com","pub_key":"%s","kind":"TRILLIANSLR"}]}`, b64(pubKeyDER)),
			wantKind: "TRILLIANSLR",
		},
		{
			desc:     "Valid Trillian SMR Source",
			body:     fmt.Sprintf(`{"entries":[{"id":"https://source.example.com","pub_key":"%s","kind":"TRILLIANSMR"}]}`, b64(pubKeyDER)),
			wantKind: "TRILLIANSMR",
		},
		{
			desc:     "Valid Gossip Hub Source",
			body:     fmt.Sprintf(`{"entries":[{"id":"https://source.example.com","pub_key":"%s","kind":"GOSSIPHUB"}]}`, b64(pubKeyDER)),
			wantKind: "GOSSIPHUB",
		},
		{
			desc:     "Valid Signed Note Source",
			body:     fmt.Sprintf(`{"entries":[{"id":"https://source.example.com","pub_key":"%s","kind":"GONOTARY"}]}`, b64(pubKeyDER)),
			wantKind: "GONOTARY",
		},
		{
			desc:     "Valid Unknown Source",
			body:     fmt.Sprintf(`{"entries":[{"id":"https://source.example.com","pub_key":"%s","kind":"bis-STH"}]}`, b64(pubKeyDER)),
			wantKind: "bis-STH",
		},
		{
			desc:    "Malformed JSON",
			body:    "fllumpf",
			wantErr: "invalid character",
		},
		{
			desc:    "GET failure",
			body:    "Utter failure",
			status:  404,
			wantErr: "404 Not Found",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ts := serveAt(t, api.GetSourceKeysPath, test.status, test.body)
			defer ts.Close()
			cl, err := client.New(ts.URL, &http.Client{}, jsonclient.Options{})
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			got, gotErr := cl.GetSourceKeys(ctx)
			if gotErr != nil {
				if len(test.wantErr) == 0 {
					t.Errorf("GetSourceKeys()=nil,%v; want _, nil", gotErr)
				} else if !strings.Contains(gotErr.Error(), test.wantErr) {
					t.Errorf("GetSourceKeys()=nil,%v; want _, err containing %q", gotErr, test.wantErr)
				}
				return
			}
			if len(test.wantErr) != 0 {
				t.Errorf("GetSourceKeys()=%v,nil; want nil, err containing %q", got, test.wantErr)
			}
			want := []*api.SourceKey{{ID: "https://source.example.com", PubKey: pubKeyDER, Kind: test.wantKind}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("GetSourceKeys()=%+v; want %+v", got, want)
			}
		})
	}
}

func TestAcceptableSource(t *testing.T) {
	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate ECDSA key: %v", err)
	}
	pubKeyDER, err := x509.MarshalPKIXPublicKey(ecdsaKey.Public())
	if err != nil {
		t.Fatalf("Failed to marshal public key to DER: %v", err)
	}
	_, otherPubKeyDER := generateKeys(t)
	tests := []struct {
		desc     string
		needle   crypto.PublicKey
		haystack []*api.SourceKey
		want     bool
	}{
		{
			desc:   "Empty Sources",
			needle: ecdsaKey.Public(),
			want:   false,
		},
		{
			desc:   "Invalid Public Key",
			needle: interface{}("bogus"),
			want:   false,
		},
		{
			desc:   "Single Source Match",
			needle: ecdsaKey.Public(),
			haystack: []*api.SourceKey{
				{PubKey: pubKeyDER},
			},
			want: true,
		},
		{
			desc:   "Multiple Source Match",
			needle: ecdsaKey.Public(),
			haystack: []*api.SourceKey{
				{PubKey: otherPubKeyDER},
				{PubKey: pubKeyDER},
			},
			want: true,
		},
		{
			desc:   "Multiple Source No Match",
			needle: ecdsaKey.Public(),
			haystack: []*api.SourceKey{
				{PubKey: otherPubKeyDER},
				{PubKey: otherPubKeyDER},
			},
			want: false,
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			got := client.AcceptableSource(test.needle, test.haystack)
			if got != test.want {
				t.Errorf("AcceptableSource()=%v, want %v", got, test.want)
			}
		})
	}
}

func TestGetLatestForSource(t *testing.T) {
	ctx := context.Background()
	want := &api.TimestampedEntry{
		SourceID:        []byte("https://a-log.com"),
		BlobData:        []byte{0x01},
		SourceSignature: []byte{0x02},
		HubTimestamp:    0x0000000012345678,
	}
	data := h2b(
		"0011" + "68747470733a2f2f612d6c6f672e636f6d" +
			"0001" + "01" +
			"0001" + "02" +
			"0000000012345678")

	tests := []struct {
		desc     string
		sourceID string
		body     string
		status   int
		wantErr  string
	}{
		{
			desc:     "Valid",
			sourceID: "https://a-log.com",
			body:     fmt.Sprintf(`{"entry":"%s"}`, b64(data)),
		},
		{
			desc:     "Source mismatch",
			sourceID: "https://another-log.com",
			body:     fmt.Sprintf(`{"entry":"%s"}`, b64(data)),
			wantErr:  "unexpected source",
		},
		{
			desc:     "Invalid trailing data",
			sourceID: "https://a-log.com",
			body:     fmt.Sprintf(`{"entry":"%s"}`, b64(append(data, 0xff))),
			wantErr:  "trailing data",
		},
		{
			desc:     "Invalid TLS data",
			sourceID: "https://a-log.com",
			body:     fmt.Sprintf(`{"entry":"%s"}`, b64([]byte{0x01, 0x02})),
			wantErr:  "tls: syntax error",
		},
		{
			desc:     "Malformed JSON",
			sourceID: "https://a-log.com",
			body:     "fllumpf",
			wantErr:  "invalid character",
		},
		{
			desc:     "GET failure",
			sourceID: "https://a-log.com",
			body:     "Utter failure",
			status:   404,
			wantErr:  "404 Not Found",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ts := serveAt(t, api.GetLatestForSourcePath, test.status, test.body)
			defer ts.Close()
			cl, err := client.New(ts.URL, &http.Client{}, jsonclient.Options{})
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			got, gotErr := cl.GetLatestForSource(ctx, test.sourceID)
			if gotErr != nil {
				if len(test.wantErr) == 0 {
					t.Errorf("GetLatestForSource()=nil,%v; want _, nil", gotErr)
				} else if !strings.Contains(gotErr.Error(), test.wantErr) {
					t.Errorf("GetLatestForSource()=nil,%v; want _, err containing %q", gotErr, test.wantErr)
				}
				return
			}
			if len(test.wantErr) != 0 {
				t.Errorf("GetLatestForSource()=%v,nil; want nil, err containing %q", got, test.wantErr)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("GetLatestForSource()=%+v; want %+v", got, want)
			}
		})
	}
}
