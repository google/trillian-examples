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

package client_test

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/client"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/crypto"
	"github.com/transparency-dev/formats/log"
	"golang.org/x/mod/sumdb/note"
)

func mustSignCPNote(t *testing.T, b string) []byte {
	t.Helper()
	s, err := note.NewSigner(crypto.TestFTPersonalityPriv)
	if err != nil {
		t.Fatalf("failed to create signer: %q", err)
	}
	n, err := note.Sign(&note.Note{Text: b}, s)
	if err != nil {
		t.Fatalf("failed to sign note: %q", err)
	}
	return n
}

func mustGetLogSigVerifier(t *testing.T) note.Verifier {
	t.Helper()
	v, err := note.NewVerifier(crypto.TestFTPersonalityPub)
	if err != nil {
		t.Fatalf("failed to create verifier: %q", err)
	}
	return v
}

func TestPublish(t *testing.T) {
	for _, test := range []struct {
		desc     string
		manifest []byte
		image    []byte
		wantErr  bool
	}{
		{
			desc:     "valid",
			manifest: []byte("Boo!"),
		}, {
			desc:     "log server fails",
			manifest: []byte("Boo!"),
			wantErr:  true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check for path prefix, trimming off leading / since it's not present in the
				// const.
				// TODO Add an index to the test case to improve coverage
				if !strings.HasPrefix(r.URL.Path[1:], api.HTTPAddFirmware) {
					t.Fatalf("Got unexpected HTTP request on %q", r.URL.Path)
				}

				if test.wantErr {
					http.Error(w, "BOOM", http.StatusInternalServerError)
					return
				}

				meta, _, err := parseAddFirmwareRequest(r)
				if err != nil {
					t.Fatalf("Failed to read multipart body: %v", err)
				}
				if diff := cmp.Diff(meta, test.manifest); len(diff) != 0 {
					t.Errorf("POSTed body with unexpected diff: %v", diff)
				}
			}))
			defer ts.Close()

			tsURL, err := url.Parse((ts.URL))
			if err != nil {
				t.Fatalf("Failed to parse test server URL: %v", err)
			}
			c := client.SubmitClient{ReadonlyClient: &client.ReadonlyClient{LogURL: tsURL}}
			err = c.PublishFirmware(test.manifest, test.image)
			switch {
			case err != nil && !test.wantErr:
				t.Fatalf("Got unexpected error %q", err)
			case err == nil && test.wantErr:
				t.Fatal("Got no error, but wanted error")
			case err != nil && test.wantErr:
				// expected error
			default:
			}
		})
	}
}

// parseAddFirmwareRequest returns the bytes for the SignedStatement, and the firmware image respectively.
// TODO(mhutchinson): For now this is a copy of the server code. de-dupe this.
func parseAddFirmwareRequest(r *http.Request) ([]byte, []byte, error) {
	h := r.Header["Content-Type"]
	if len(h) == 0 {
		return nil, nil, fmt.Errorf("no content-type header")
	}

	mediaType, mediaParams, err := mime.ParseMediaType(h[0])
	if err != nil {
		return nil, nil, err
	}
	if !strings.HasPrefix(mediaType, "multipart/") {
		return nil, nil, fmt.Errorf("expecting mime multipart body")
	}
	boundary := mediaParams["boundary"]
	if len(boundary) == 0 {
		return nil, nil, fmt.Errorf("invalid mime multipart header - no boundary specified")
	}
	mr := multipart.NewReader(r.Body, boundary)

	// Get firmware statement (JSON)
	p, err := mr.NextPart()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find firmware statement in request body: %v", err)
	}
	rawJSON, err := io.ReadAll(p)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read body of firmware statement: %v", err)
	}

	// Get firmware binary image
	p, err = mr.NextPart()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find firmware image in request body: %v", err)
	}
	image, err := io.ReadAll(p)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read body of firmware image: %v", err)
	}
	return rawJSON, image, nil
}

func TestGetCheckpoint(t *testing.T) {
	for _, test := range []struct {
		desc    string
		body    []byte
		want    api.LogCheckpoint
		wantErr bool
	}{
		{
			desc: "valid 1",
			body: mustSignCPNote(t, "Firmware Transparency Log\n1\nEjQ=\n123\n"),
			want: api.LogCheckpoint{
				Checkpoint: log.Checkpoint{
					Origin: "Firmware Transparency Log",
					Size:   1,
					Hash:   []byte{0x12, 0x34},
				},
				TimestampNanos: 123,
			},
		}, {
			desc: "valid 2",
			body: mustSignCPNote(t, "Firmware Transparency Log\n10\nNBI=\n1230\n"),
			want: api.LogCheckpoint{
				Checkpoint: log.Checkpoint{
					Origin: "Firmware Transparency Log",
					Size:   10,
					Hash:   []byte{0x34, 0x12},
				},
				TimestampNanos: 1230,
			},
		}, {
			desc:    "garbage",
			body:    []byte(`garbage`),
			wantErr: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, api.HTTPGetRoot) {
					t.Fatalf("Got unexpected HTTP request on %q", r.URL.Path)
				}
				fmt.Fprint(w, string(test.body))
			}))
			defer ts.Close()

			tsURL, err := url.Parse((ts.URL))
			if err != nil {
				t.Fatalf("Failed to parse test server URL: %v", err)
			}
			c := client.ReadonlyClient{
				LogURL:         tsURL,
				LogSigVerifier: mustGetLogSigVerifier(t),
			}
			cp, err := c.GetCheckpoint()
			switch {
			case err != nil && !test.wantErr:
				t.Fatalf("Got unexpected error %q", err)
			case err == nil && test.wantErr:
				t.Fatal("Got no error, but wanted error")
			case err != nil && test.wantErr:
				// expected error
			default:
				// Ignore the envelope data:
				cp.Envelope = nil
				if d := cmp.Diff(*cp, test.want); len(d) != 0 {
					t.Fatalf("Got checkpoint with diff: %s", d)
				}
			}
		})
	}
}

func TestGetInclusion(t *testing.T) {
	cp := api.LogCheckpoint{
		Checkpoint: log.Checkpoint{
			Size: 30,
		},
	}
	for _, test := range []struct {
		desc    string
		body    string
		want    api.InclusionProof
		wantErr bool
	}{
		{
			desc: "valid 1",
			body: `{ "LeafIndex": 2, "Proof": ["qg==", "uw==", "zA=="]}`,
			want: api.InclusionProof{LeafIndex: 2, Proof: [][]byte{{0xAA}, {0xBB}, {0xCC}}},
		}, {
			desc: "valid 2",
			body: `{ "LeafIndex": 20, "Proof": ["3Q==", "7g=="]}`,
			want: api.InclusionProof{LeafIndex: 20, Proof: [][]byte{{0xDD}, {0xEE}}},
		}, {
			desc:    "garbage",
			body:    `garbage`,
			wantErr: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check for path prefix, trimming off leading / since it's not present in the
				// const.
				// TODO Add an index to the test case to improve coverage
				if !strings.HasPrefix(r.URL.Path[1:], api.HTTPGetInclusion) {
					t.Fatalf("Got unexpected HTTP request on %q", r.URL.Path)
				}
				fmt.Fprintln(w, test.body)
			}))
			defer ts.Close()

			tsURL, err := url.Parse((ts.URL))
			if err != nil {
				t.Fatalf("Failed to parse test server URL: %v", err)
			}
			c := client.ReadonlyClient{LogURL: tsURL}
			ip, err := c.GetInclusion([]byte{}, cp)
			switch {
			case err != nil && !test.wantErr:
				t.Fatalf("Got unexpected error %q", err)
			case err == nil && test.wantErr:
				t.Fatal("Got no error, but wanted error")
			case err != nil && test.wantErr:
				// expected error
			default:
				if d := cmp.Diff(ip, test.want); len(d) != 0 {
					t.Fatalf("Got checkpoint with diff: %s", d)
				}
			}
		})
	}
}

func TestGetManifestAndProof(t *testing.T) {
	for _, test := range []struct {
		desc    string
		body    string
		want    api.InclusionProof
		wantErr bool
	}{
		{
			desc: "valid 1",
			body: `{ "Value":"EjQ=", "Proof": ["qg==", "uw==", "zA=="]}`,
			want: api.InclusionProof{Value: []byte{0x12, 0x34}, Proof: [][]byte{{0xAA}, {0xBB}, {0xCC}}},
		}, {
			desc: "valid 2",
			body: `{ "Value":"NBI=","Proof": ["3Q==", "7g=="]}`,
			want: api.InclusionProof{Value: []byte{0x34, 0x12}, Proof: [][]byte{{0xDD}, {0xEE}}},
		}, {
			desc:    "garbage",
			body:    `garbage`,
			wantErr: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check for path prefix, trimming off leading / since it's not present in the
				// const.
				// TODO Add an index to the test case to improve coverage
				if !strings.HasPrefix(r.URL.Path[1:], api.HTTPGetManifestEntryAndProof) {
					t.Fatalf("Got unexpected HTTP request on %q", r.URL.Path)
				}
				fmt.Fprintln(w, test.body)
			}))
			defer ts.Close()

			tsURL, err := url.Parse((ts.URL))
			if err != nil {
				t.Fatalf("Failed to parse test server URL: %v", err)
			}
			c := client.ReadonlyClient{LogURL: tsURL}
			ip, err := c.GetManifestEntryAndProof(api.GetFirmwareManifestRequest{Index: 0, TreeSize: 0})
			switch {
			case err != nil && !test.wantErr:
				t.Fatalf("Got unexpected error %q", err)
			case err == nil && test.wantErr:
				t.Fatal("Got no error, but wanted error")
			case err != nil && test.wantErr:
				// expected error
			default:
				if d := cmp.Diff(*ip, test.want); len(d) != 0 {
					t.Fatalf("Got response with diff: %s", d)
				}
			}
		})
	}
}

func TestGetConsistency(t *testing.T) {
	for _, test := range []struct {
		desc    string
		body    string
		From    uint64
		To      uint64
		want    api.ConsistencyProof
		wantErr bool
	}{
		{
			desc: "valid 1",
			body: `{"Proof": ["qg==", "uw==", "zA=="]}`,
			From: 0,
			To:   1,
			want: api.ConsistencyProof{Proof: [][]byte{{0xAA}, {0xBB}, {0xCC}}},
		}, {
			desc: "valid 2",
			body: `{"Proof": ["3Q==", "7g=="]}`,
			From: 1,
			To:   2,
			want: api.ConsistencyProof{Proof: [][]byte{{0xDD}, {0xEE}}},
		}, {
			desc:    "garbage",
			body:    `garbage`,
			wantErr: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check for path prefix, trimming off leading / since it's not present in the
				// const.
				// TODO Add an index to the test case to improve coverage
				if !strings.HasPrefix(r.URL.Path[1:], api.HTTPGetConsistency) {
					t.Fatalf("Got unexpected HTTP request on %q", r.URL.Path)
				}
				fmt.Fprintln(w, test.body)
			}))
			defer ts.Close()

			tsURL, err := url.Parse((ts.URL))
			if err != nil {
				t.Fatalf("Failed to parse test server URL: %v", err)
			}
			c := client.ReadonlyClient{LogURL: tsURL}
			cp, err := c.GetConsistencyProof(api.GetConsistencyRequest{From: test.From, To: test.To})
			switch {
			case err != nil && !test.wantErr:
				t.Fatalf("Got unexpected error %q", err)
			case err == nil && test.wantErr:
				t.Fatal("Got no error, but wanted error")
			case err != nil && test.wantErr:
				// expected error
			default:
				if d := cmp.Diff(*cp, test.want); len(d) != 0 {
					t.Fatalf("Got response with diff: %s", d)
				}
			}
		})
	}
}

func TestGetFirmwareImage(t *testing.T) {
	knownHash := []byte("knownhash")
	for _, test := range []struct {
		desc      string
		hash      []byte
		body      []byte
		isUnknown bool
		want      []byte
		wantErr   bool
	}{
		{
			desc: "valid",
			hash: knownHash,
			body: []byte("body"),
			want: []byte("body"),
		}, {
			desc:      "not found",
			hash:      []byte("never heard of it"),
			isUnknown: true,
			wantErr:   true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check for path prefix, trimming off leading / since it's not present in the
				// const.
				// TODO Add an index to the test case to improve coverage
				if !strings.HasPrefix(r.URL.Path[1:], api.HTTPGetFirmwareImage) {
					t.Fatalf("Got unexpected HTTP request on %q", r.URL.Path)
				}
				if test.isUnknown {
					http.Error(w, "unknown", http.StatusNotFound)
					return
				}
				w.Write(test.body)
			}))
			defer ts.Close()

			tsURL, err := url.Parse((ts.URL))
			if err != nil {
				t.Fatalf("Failed to parse test server URL: %v", err)
			}
			c := client.ReadonlyClient{LogURL: tsURL}
			img, err := c.GetFirmwareImage(test.hash)
			switch {
			case err != nil && !test.wantErr:
				t.Fatalf("Got unexpected error %q", err)
			case err == nil && test.wantErr:
				t.Fatal("Got no error, but wanted error")
			case err != nil && test.wantErr:
				// expected error
			default:
				if got, want := img, test.want; !bytes.Equal(got, want) {
					t.Fatalf("Got body %q, want %q", got, want)
				}
			}
		})
	}
}
