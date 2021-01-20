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

package http

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gomock "github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/crypto"
	"github.com/google/trillian/types"
	"github.com/gorilla/mux"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRoot(t *testing.T) {
	for _, test := range []struct {
		desc     string
		root     types.LogRootV1
		wantBody string
	}{
		{
			desc:     "valid 1",
			root:     types.LogRootV1{TreeSize: 1, TimestampNanos: 123, RootHash: []byte{0x12, 0x34}},
			wantBody: `{"TreeSize":1,"RootHash":"EjQ=","TimestampNanos":123}`,
		}, {
			desc:     "valid 2",
			root:     types.LogRootV1{TreeSize: 10, TimestampNanos: 1230, RootHash: []byte{0x34, 0x12}},
			wantBody: `{"TreeSize":10,"RootHash":"NBI=","TimestampNanos":1230}`,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mt := NewMockTrillian(ctrl)
			server := NewServer(mt, FakeCAS{})

			mt.EXPECT().Root().Return(&test.root)

			ts := httptest.NewServer(http.HandlerFunc(server.getRoot))
			defer ts.Close()

			client := ts.Client()
			resp, err := client.Get(ts.URL)
			if err != nil {
				t.Errorf("error response: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("status code not OK: %v", resp.StatusCode)
			}
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Errorf("failed to read body: %v", err)
			}
			if string(body) != test.wantBody {
				t.Errorf("got '%s' want '%s'", string(body), test.wantBody)
			}
		})
	}
}

func b64Decode(t *testing.T, b64 string) []byte {
	t.Helper()
	st, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("b64 decoding failed: %v", err)
	}
	return st
}
func TestAddFirmware(t *testing.T) {
	st := b64Decode(t, "eyJEZXZpY2VJRCI6IlRhbGtpZVRvYXN0ZXIiLCJGaXJtd2FyZVJldmlzaW9uIjoxLCJGaXJtd2FyZUltYWdlU0hBNTEyIjoiMTRxN0JVSnphR1g1UndSU0ZnbkNNTnJBT2k4Mm5RUTZ3aExXa3p1UlFRNEdPWjQzK2NYTWlFTnFNWE56TU1ISTdNc3NMNTgzVFdMM0ZrTXFNdFVQckE9PSIsIkV4cGVjdGVkRmlybXdhcmVNZWFzdXJlbWVudCI6IiIsIkJ1aWxkVGltZXN0YW1wIjoiMjAyMC0xMS0xN1QxMzozMDoxNFoifQ==")
	sign, err := crypto.SignMessage(st)
	if err != nil {
		t.Fatalf("signing failed, bailing out!: %v", err)
	}
	statement := api.FirmwareStatement{Metadata: st, Signature: sign}
	js, err := json.Marshal(statement)
	if err != nil {
		t.Fatalf("marshaling failed, bailing out!: %v", err)
	}

	s := string(js)
	for _, test := range []struct {
		desc         string
		body         string
		trillianErr  error
		wantManifest string
		wantStatus   int
	}{
		{
			desc:       "malformed request",
			body:       "garbage",
			wantStatus: http.StatusBadRequest,
		}, {
			desc: "valid request",
			body: strings.Join([]string{"--mimeisfunlolol",
				"Content-Type: application/json",
				"",
				s,
				"--mimeisfunlolol",
				"Content-Type: application/octet-stream",
				"",
				"hi",
				"",
				"--mimeisfunlolol--",
				"",
			}, "\n"),
			wantManifest: s,
			wantStatus:   http.StatusOK,
		}, {
			desc: "firmware image does not match manifest",
			body: strings.Join([]string{"--mimeisfunlolol",
				"Content-Type: application/json",
				"",
				s,
				"--mimeisfunlolol",
				"Content-Type: application/octet-stream",
				"",
				"THIS HAS A DIFFERENT HASH THAN EXPECTED",
				"",
				"--mimeisfunlolol--",
				"",
			}, "\n"),
			wantManifest: s,
			wantStatus:   http.StatusBadRequest,
		}, {
			desc: "valid request but trillian failure",
			body: strings.Join([]string{"--mimeisfunlolol",
				"Content-Type: application/json",
				"",
				s,
				"--mimeisfunlolol",
				"Content-Type: application/octet-stream",
				"",
				"hi",
				"",
				"--mimeisfunlolol--",
				"",
			}, "\n"),
			wantManifest: s,
			trillianErr:  errors.New("boom"),
			wantStatus:   http.StatusInternalServerError,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mt := NewMockTrillian(ctrl)
			server := NewServer(mt, FakeCAS{})

			mt.EXPECT().AddSignedStatement(gomock.Any(), gomock.Eq([]byte(test.wantManifest))).
				Return(test.trillianErr)

			r := mux.NewRouter()
			server.RegisterHandlers(r)
			ts := httptest.NewServer(r)
			defer ts.Close()

			client := ts.Client()
			url := fmt.Sprintf("%s/%s", ts.URL, api.HTTPAddFirmware)
			resp, err := client.Post(url, "multipart/form-data; boundary=mimeisfunlolol", strings.NewReader(test.body))
			if err != nil {
				t.Errorf("error response: %v", err)
			}
			if got, want := resp.StatusCode, test.wantStatus; got != want {
				t.Errorf("status code got != want (%d, %d):", got, want)
			}
		})
	}
}

func TestGetConsistency(t *testing.T) {
	root := types.LogRootV1{TreeSize: 24, TimestampNanos: 123, RootHash: []byte{0x12, 0x34}}
	for _, test := range []struct {
		desc             string
		from, to         int
		wantFrom, wantTo uint64
		trillianProof    [][]byte
		trillianErr      error
		wantStatus       int
		wantBody         string
	}{
		{
			desc:          "valid request",
			from:          1,
			to:            24,
			wantFrom:      1,
			wantTo:        24,
			trillianProof: [][]byte{[]byte("pr"), []byte("oo"), []byte("f!")},
			wantStatus:    http.StatusOK,
			wantBody:      `{"Proof":["cHI=","b28=","ZiE="]}`,
		}, {
			desc:       "ToSize bigger than tree size",
			from:       1,
			to:         25,
			wantStatus: http.StatusBadRequest,
		}, {
			desc:       "FromSize too large",
			from:       15,
			to:         12,
			wantStatus: http.StatusBadRequest,
		}, {
			desc:        "valid request but trillian failure",
			from:        11,
			to:          15,
			wantFrom:    11,
			wantTo:      15,
			trillianErr: errors.New("boom"),
			wantStatus:  http.StatusInternalServerError,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mt := NewMockTrillian(ctrl)
			server := NewServer(mt, FakeCAS{})
			mt.EXPECT().Root().
				Return(&root)

			mt.EXPECT().ConsistencyProof(gomock.Any(), gomock.Eq(test.wantFrom), gomock.Eq(test.wantTo)).
				Return(test.trillianProof, test.trillianErr)

			r := mux.NewRouter()
			server.RegisterHandlers(r)
			ts := httptest.NewServer(r)
			defer ts.Close()
			url := fmt.Sprintf("%s/%s/from/%d/to/%d", ts.URL, api.HTTPGetConsistency, test.from, test.to)

			client := ts.Client()
			resp, err := client.Get(url)
			if err != nil {
				t.Errorf("error response: %v", err)
			}
			if got, want := resp.StatusCode, test.wantStatus; got != want {
				t.Errorf("status code got != want (%d, %d)", got, want)
			}
			if len(test.wantBody) > 0 {
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					t.Errorf("failed to read body: %v", err)
				}
				if got, want := string(body), test.wantBody; got != test.wantBody {
					t.Errorf("got '%s' want '%s'", got, want)
				}
			}
		})
	}
}

func TestGetManifestEntries(t *testing.T) {
	root := types.LogRootV1{TreeSize: 24, TimestampNanos: 123, RootHash: []byte{0x12, 0x34}}
	for _, test := range []struct {
		desc                string
		index               int
		treeSize            int
		wantIndex, wantSize uint64
		trillianData        []byte
		trillianProof       [][]byte
		trillianErr         error
		wantStatus          int
		wantBody            string
	}{
		{
			desc:          "valid request",
			index:         1,
			treeSize:      24,
			wantIndex:     1,
			wantSize:      24,
			trillianData:  []byte("leafdata"),
			trillianProof: [][]byte{[]byte("pr"), []byte("oo"), []byte("f!")},
			wantStatus:    http.StatusOK,
			wantBody:      `{"Value":"bGVhZmRhdGE=","LeafIndex":1,"Proof":["cHI=","b28=","ZiE="]}`,
		}, {
			desc:       "TreeSize bigger than golden tree size",
			index:      1,
			treeSize:   29,
			wantStatus: http.StatusBadRequest,
		}, {
			desc:       "LeafIndex larger than tree size",
			index:      1,
			treeSize:   0,
			wantStatus: http.StatusBadRequest,
		}, {
			desc:       "LeafIndex equal to tree size",
			index:      4,
			treeSize:   4,
			wantStatus: http.StatusBadRequest,
		}, {
			desc:        "valid request but trillian failure",
			index:       1,
			treeSize:    24,
			wantIndex:   1,
			wantSize:    24,
			trillianErr: errors.New("boom"),
			wantStatus:  http.StatusInternalServerError,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mt := NewMockTrillian(ctrl)
			server := NewServer(mt, FakeCAS{})

			mt.EXPECT().Root().
				Return(&root)

			mt.EXPECT().FirmwareManifestAtIndex(gomock.Any(), gomock.Eq(test.wantIndex), gomock.Eq(test.wantSize)).
				Return(test.trillianData, test.trillianProof, test.trillianErr)

			r := mux.NewRouter()
			server.RegisterHandlers(r)
			ts := httptest.NewServer(r)
			defer ts.Close()
			url := fmt.Sprintf("%s/%s/at/%d/in-tree-of/%d", ts.URL, api.HTTPGetManifestEntryAndProof, test.index, test.treeSize)

			client := ts.Client()
			resp, err := client.Get(url)
			if err != nil {
				t.Errorf("error response: %v", err)
			}
			if got, want := resp.StatusCode, test.wantStatus; got != want {
				t.Errorf("status code got != want (%d, %d)", got, want)
			}
			if len(test.wantBody) > 0 {
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					t.Errorf("failed to read body: %v", err)
				}
				if got, want := string(body), test.wantBody; got != test.wantBody {
					t.Errorf("got '%s' want '%s'", got, want)
				}
			}
		})
	}
}

func TestGetInclusionProofByHash(t *testing.T) {
	root := types.LogRootV1{TreeSize: 24, TimestampNanos: 123, RootHash: []byte{0x12, 0x34}}
	for _, test := range []struct {
		desc          string
		hash          []byte
		treeSize      int
		trillianIndex uint64
		trillianProof [][]byte
		trillianErr   error
		wantStatus    int
	}{
		{
			desc:          "valid request",
			hash:          []byte("a good leaf hash"),
			treeSize:      24,
			trillianProof: [][]byte{[]byte("pr"), []byte("oo"), []byte("f!")},
			trillianIndex: 4,
			wantStatus:    http.StatusOK,
		}, {
			desc:       "TreeSize bigger than golden tree size",
			hash:       []byte("a good leaf hash"),
			treeSize:   29,
			wantStatus: http.StatusBadRequest,
		}, {
			desc:        "unknown leafhash",
			hash:        []byte("made up leaf hash"),
			treeSize:    24,
			trillianErr: status.Error(codes.NotFound, "never heard of it, mate"),
			wantStatus:  http.StatusNotFound,
		}, {
			desc:        "valid request but trillian failure",
			hash:        []byte("a good leaf hash"),
			treeSize:    24,
			trillianErr: errors.New("boom"),
			wantStatus:  http.StatusInternalServerError,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mt := NewMockTrillian(ctrl)
			server := NewServer(mt, FakeCAS{})

			mt.EXPECT().Root().
				Return(&root)

			mt.EXPECT().InclusionProofByHash(gomock.Any(), gomock.Eq(test.hash), gomock.Eq(root.TreeSize)).
				Return(test.trillianIndex, test.trillianProof, test.trillianErr)

			r := mux.NewRouter()
			server.RegisterHandlers(r)
			ts := httptest.NewServer(r)
			defer ts.Close()
			url := fmt.Sprintf("%s/%s/for-leaf-hash/%s/in-tree-of/%d", ts.URL, api.HTTPGetInclusion, base64.URLEncoding.EncodeToString(test.hash), test.treeSize)

			client := ts.Client()
			resp, err := client.Get(url)
			if err != nil {
				t.Errorf("error response: %v", err)
			}
			if got, want := resp.StatusCode, test.wantStatus; got != want {
				t.Errorf("status code got != want (%d, %d)", got, want)
			}
			if test.wantStatus == http.StatusOK {
				// If we're expecting a good response then check that all values got passed through ok
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("failed to read body: %v", err)
				}
				wantBody := api.InclusionProof{
					LeafIndex: test.trillianIndex,
					Proof:     test.trillianProof,
				}
				var gotBody api.InclusionProof
				if err := json.Unmarshal(body, &gotBody); err != nil {
					t.Fatalf("got invalid json response: %q", err)
				}
				if diff := cmp.Diff(gotBody, wantBody); len(diff) > 0 {
					t.Errorf("got response with diff %q", diff)
				}
			}
		})
	}
}

type FakeCAS map[string][]byte

func (f FakeCAS) Store(key, image []byte) error {
	f[string(key)] = image
	return nil
}

func (f FakeCAS) Retrieve(key []byte) ([]byte, error) {
	if image, ok := f[string(key)]; ok {
		return image, nil
	}
	return nil, errors.New("nope")
}
