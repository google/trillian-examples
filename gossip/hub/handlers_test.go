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

package hub

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/certificate-transparency-go/tls"
	"github.com/google/certificate-transparency-go/trillian/mockclient"
	"github.com/google/trillian"
	"github.com/google/trillian-examples/gossip/api"
	"github.com/google/trillian-examples/gossip/hub/configpb"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ct "github.com/google/certificate-transparency-go"
)

const (
	testTreeID      = int64(55)
	testDeadline    = 500 * time.Millisecond
	testSourceID    = "https://the-log.example.com"
	testCTSourceID  = "https://rfc6962.example.com"
	testSLRSourceID = "https://trillian-log.example.com"
)

var (
	testSourceKey       *ecdsa.PrivateKey
	testSourcePubKeyDER []byte
	testSources         []*api.SourceKey
)

func init() {
	// Make a key pair to use when creating source log data to publish to a hub.
	var err error
	testSourceKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(fmt.Sprintf("Failed to generate ECDSA key: %v", err))
	}
	testSourcePubKeyDER, err = x509.MarshalPKIXPublicKey(testSourceKey.Public())
	if err != nil {
		panic(fmt.Sprintf("Failed to marshal public key to DER: %v", err))
	}
	testSources = []*api.SourceKey{
		{ID: testSourceID, PubKey: testSourcePubKeyDER, Kind: api.UnknownKind},
		{ID: testCTSourceID, PubKey: testSourcePubKeyDER, Kind: api.RFC6962STHKind},
		{ID: testSLRSourceID, PubKey: testSourcePubKeyDER, Kind: api.TrillianSLRKind},
	}
}

type handlerTestInfo struct {
	mockCtrl *gomock.Controller
	client   *mockclient.MockTrillianLogClient
	hi       *hubInfo
}

// setupTest creates mock objects.  Caller should invoke info.mockCtrl.Finish().
func setupTest(t *testing.T, signer crypto.Signer) handlerTestInfo {
	t.Helper()

	mockCtrl := gomock.NewController(t)
	client := mockclient.NewMockTrillianLogClient(mockCtrl)
	iOpts := InstanceOptions{
		Deadline:      testDeadline,
		MetricFactory: monitoring.InertMetricFactory{},
	}

	// Make a private key for the hub to use.
	if signer == nil {
		var err error
		signer, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("Failed to generate ECDSA key: %v", err)
		}
	}

	cryptoMap := map[string]sourceCryptoInfo{
		testSourceID: {
			pubKeyData: testSourcePubKeyDER,
			pubKey:     testSourceKey.Public(),
			hasher:     crypto.SHA256,
		},
		testCTSourceID: {
			pubKeyData: testSourcePubKeyDER,
			pubKey:     testSourceKey.Public(),
			hasher:     crypto.SHA256,
			kind:       configpb.TrackedSource_RFC6962STH,
		},
		testSLRSourceID: {
			pubKeyData: testSourcePubKeyDER,
			pubKey:     testSourceKey.Public(),
			hasher:     crypto.SHA256,
			kind:       configpb.TrackedSource_TRILLIANSLR,
		},
	}
	return handlerTestInfo{
		mockCtrl: mockCtrl,
		client:   client,
		hi:       newHubInfo(testTreeID, "testhub", client, signer, cryptoMap, iOpts),
	}
}

func TestHandlers(t *testing.T) {
	path := "/test-prefix/gossip/v0/add-signed-blob"
	var tests = []string{
		"/test-prefix/",
		"test-prefix/",
		"/test-prefix",
		"test-prefix",
	}
	info := setupTest(t, nil)
	defer info.mockCtrl.Finish()

	for _, test := range tests {
		handlers := info.hi.Handlers(test)
		if h, ok := handlers[path]; !ok {
			t.Errorf("Handlers(%s)[%q]=%+v; want _", test, path, h)
		} else if h.Name != "add-signed-blob" {
			t.Errorf("Handlers(%s)[%q].Name=%q; want 'add-signed-blob'", test, path, h.Name)
		}
		// Check each entrypoint has a handler
		if got, want := len(handlers), len(Entrypoints); got != want {
			t.Errorf("len(Handlers(%s))=%d; want %d", test, got, want)
		}
	outer:
		for _, ep := range Entrypoints {
			for _, v := range handlers {
				if v.Name == ep {
					continue outer
				}
			}
			t.Errorf("Handlers(%s) missing entry with .Name=%q", test, ep)
		}
	}
}

func TestServeHTTP(t *testing.T) {
	info := &hubInfo{logID: 55, hubPrefix: "the-hub", opts: InstanceOptions{Deadline: 100 * time.Millisecond}}
	tests := []struct {
		desc    string
		req     *http.Request
		handler func(context.Context, *hubInfo, http.ResponseWriter, *http.Request) (int, error)
		want    int
	}{
		{
			desc: "Wrong Method",
			req:  httptest.NewRequest("POST", "http://example.com/test", nil),
			want: http.StatusMethodNotAllowed,
		},
		{
			desc: "Malformed Parameters",
			req:  httptest.NewRequest("GET", "http://example.com/test?key=value%", nil),
			want: http.StatusBadRequest,
		},
		{
			desc: "Handler Error",
			req:  httptest.NewRequest("GET", "http://example.com/test", nil),
			handler: func(_ context.Context, _ *hubInfo, w http.ResponseWriter, _ *http.Request) (int, error) {
				return http.StatusTeapot, errors.New("test")
			},
			want: http.StatusTeapot,
		},
		{
			desc: "Handler Errorless non-200",
			req:  httptest.NewRequest("GET", "http://example.com/test", nil),
			handler: func(_ context.Context, _ *hubInfo, w http.ResponseWriter, _ *http.Request) (int, error) {
				return http.StatusTeapot, nil
			},
			want: http.StatusInternalServerError,
		},
		{
			desc: "Handler OK",
			req:  httptest.NewRequest("GET", "http://example.com/test", nil),
			handler: func(_ context.Context, _ *hubInfo, w http.ResponseWriter, _ *http.Request) (int, error) {
				return http.StatusOK, nil
			},
			want: http.StatusOK,
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			handler := AppHandler{info: info, handler: test.handler, Name: "test", method: http.MethodGet}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, test.req)
			got := w.Result().StatusCode
			if got != test.want {
				t.Errorf("ServeHTTP(%v)=%d, want %d", test.req, got, test.want)
			}
		})
	}
}

func TestCTSTHIsLater(t *testing.T) {
	sthData := func(sz, ts uint64) []byte {
		th := ct.TreeHeadSignature{
			Version:       ct.V1,
			SignatureType: ct.TreeHashSignatureType,
			TreeSize:      sz,
			Timestamp:     ts,
		}
		data, _ := tls.Marshal(th)
		return data
	}
	tests := []struct {
		desc       string
		prev, curr []byte
		want       bool
	}{
		{
			desc: "smaller",
			prev: sthData(100, 10),
			curr: sthData(90, 10),
			want: false,
		},
		{
			desc: "bigger",
			prev: sthData(100, 10),
			curr: sthData(110, 10),
			want: true,
		},
		{
			desc: "same-size-newer",
			prev: sthData(100, 10),
			curr: sthData(100, 12),
			want: true,
		},
		{
			desc: "same-size-older",
			prev: sthData(100, 10),
			curr: sthData(100, 9),
			want: false,
		},
		{
			desc: "same-everything",
			prev: sthData(100, 10),
			curr: sthData(100, 10),
			want: false,
		},
		{
			desc: "malformed-prev",
			prev: []byte{0x01},
			curr: sthData(100, 10),
			want: true,
		},
		{
			desc: "malformed-curr",
			prev: sthData(100, 10),
			curr: []byte{0x01},
			want: false,
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			prev := &api.TimestampedEntry{BlobData: test.prev}
			curr := &api.TimestampedEntry{BlobData: test.curr}
			got := ctSTHIsLater(prev, curr)
			if got != test.want {
				t.Errorf("ctSTHIsLater()=%v, want %v", got, test.want)
			}
		})
	}
}

func TestTrillianSLRIsLater(t *testing.T) {
	slrData := func(sz, ts uint64) []byte {
		lr := types.LogRoot{
			Version: 1,
			V1: &types.LogRootV1{
				TreeSize:       sz,
				TimestampNanos: ts,
			},
		}
		data, _ := tls.Marshal(lr)
		return data
	}

	tests := []struct {
		desc       string
		prev, curr []byte
		want       bool
	}{
		{
			desc: "smaller",
			prev: slrData(100, 10),
			curr: slrData(90, 10),
			want: false,
		},
		{
			desc: "bigger",
			prev: slrData(100, 10),
			curr: slrData(110, 10),
			want: true,
		},
		{
			desc: "same-size-newer",
			prev: slrData(100, 10),
			curr: slrData(100, 12),
			want: true,
		},
		{
			desc: "same-size-older",
			prev: slrData(100, 10),
			curr: slrData(100, 9),
			want: false,
		},
		{
			desc: "same-everything",
			prev: slrData(100, 10),
			curr: slrData(100, 10),
			want: false,
		},
		{
			desc: "malformed-prev",
			prev: []byte{0x01},
			curr: slrData(100, 10),
			want: true,
		},
		{
			desc: "malformed-curr",
			prev: slrData(100, 10),
			curr: []byte{0x01},
			want: false,
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			prev := &api.TimestampedEntry{BlobData: test.prev}
			curr := &api.TimestampedEntry{BlobData: test.curr}
			got := trillianSLRIsLater(prev, curr)
			if got != test.want {
				t.Errorf("ctSTHIsLater(%x, %x)=%v, want %v", test.prev, test.curr, got, test.want)
			}
		})
	}
}

func makeSLR(t *testing.T, timestamp, treeSize int64, hash []byte) *trillian.SignedLogRoot {
	t.Helper()
	root := types.LogRootV1{
		TreeSize:       uint64(treeSize),
		RootHash:       hash,
		TimestampNanos: uint64(timestamp),
		Revision:       1,
	}
	rootData, err := root.MarshalBinary()
	if err != nil {
		t.Fatalf("Failed to marshal log root: %v", err)
	}
	return &trillian.SignedLogRoot{
		LogRoot: rootData,
		// Deprecated fields filled in too:
		TimestampNanos: timestamp,
		TreeSize:       treeSize,
		RootHash:       hash,
	}
}

func makeGetSLRRsp(t *testing.T, timestamp, treeSize int64, hash []byte) *trillian.GetLatestSignedLogRootResponse {
	return &trillian.GetLatestSignedLogRootResponse{SignedLogRoot: makeSLR(t, timestamp, treeSize, hash)}
}

// signData returns data, signature-over-data as a 2 element slice of base-64 encoded strings.
func signData(t *testing.T, data []byte) []interface{} {
	digest := sha256.Sum256(data)
	sig, err := testSourceKey.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		t.Fatalf("Failed to sign data: %v", err)
	}
	return []interface{}{base64.StdEncoding.EncodeToString(data), base64.StdEncoding.EncodeToString(sig)}
}

func TestAddSignedBlob(t *testing.T) {
	ctx := context.Background()

	th := ct.TreeHeadSignature{
		Version:       ct.V1,
		SignatureType: ct.TreeHashSignatureType,
		Timestamp:     1000000,
		TreeSize:      100,
	}
	rand.Read(th.SHA256RootHash[:])
	data, err := tls.Marshal(th)
	if err != nil {
		t.Fatalf("failed to create fake CT tree head: %v", err)
	}
	lr := types.LogRoot{
		Version: 1,
		V1: &types.LogRootV1{
			TreeSize:       100,
			RootHash:       make([]byte, sha256.Size),
			TimestampNanos: 1000000000,
			Revision:       42,
		},
	}
	rand.Read(lr.V1.RootHash)
	slrData, err := tls.Marshal(lr)
	if err != nil {
		t.Fatalf("failed to create fake Trillian log root: %v", err)
	}

	digest := sha256.Sum256(data)
	sig, err := testSourceKey.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		t.Fatalf("Failed to sign test data: %v", err)
	}
	logHead := api.AddSignedBlobRequest{
		SourceID:        testSourceID,
		BlobData:        data,
		SourceSignature: sig,
	}
	logHeadJSON, err := json.Marshal(logHead)
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}
	entry := api.TimestampedEntry{
		SourceID:        []byte(testSourceID),
		BlobData:        data,
		SourceSignature: sig,
	}
	leafValueData, err := tls.Marshal(entry)
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}
	leafIdentityHash := sha256.Sum256(leafValueData)

	tests := []struct {
		desc       string
		body       string
		rpcRsp     *trillian.QueueLeavesResponse
		rpcErr     error
		forceRPC   bool
		signer     crypto.Signer
		wantStatus int
		wantErr    string
	}{
		{
			desc:       "malformed-request",
			body:       "nostalgic llama",
			wantStatus: http.StatusBadRequest,
			wantErr:    "failed to parse add-signed-blob",
		},
		{
			desc:       "unknown-source",
			body:       `{"source_id":"http://wrong-log.com","blob_data":"AAAA","src_signature":"AAAA"}`,
			wantStatus: http.StatusNotFound,
			wantErr:    "unknown source",
		},
		{
			desc:       "invalid-signature",
			body:       `{"source_id":"https://the-log.example.com","blob_data":"AAAA","src_signature":"AAAA"}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    "failed to validate signature",
		},
		{
			desc:       "not-a-ct-sth",
			body:       fmt.Sprintf(`{"source_id":"https://rfc6962.example.com","blob_data":"%s","src_signature":"%s"}`, signData(t, []byte{0x00})...),
			wantStatus: http.StatusBadRequest,
			wantErr:    "failed to parse as tree head",
		},
		{
			desc:       "trailing-ct-sth",
			body:       fmt.Sprintf(`{"source_id":"https://rfc6962.example.com","blob_data":"%s","src_signature":"%s"}`, signData(t, append(data, 0xff))...),
			wantStatus: http.StatusBadRequest,
			wantErr:    "1 bytes of trailing data",
		},
		{
			desc:       "not-an-slr",
			body:       fmt.Sprintf(`{"source_id":"https://trillian-log.example.com","blob_data":"%s","src_signature":"%s"}`, signData(t, []byte{0x00})...),
			wantStatus: http.StatusBadRequest,
			wantErr:    "failed to parse as log root",
		},
		{
			desc:       "trailing-slr",
			body:       fmt.Sprintf(`{"source_id":"https://trillian-log.example.com","blob_data":"%s","src_signature":"%s"}`, signData(t, append(slrData, 0xff))...),
			wantStatus: http.StatusBadRequest,
			wantErr:    "1 bytes of trailing data after log root",
		},
		{
			desc:       "backend-failure",
			body:       string(logHeadJSON),
			rpcErr:     errors.New("backendfailure"),
			wantStatus: http.StatusInternalServerError,
			wantErr:    "backendfailure",
		},
		{
			desc:       "missing-backend-rsp",
			body:       string(logHeadJSON),
			forceRPC:   true,
			wantStatus: http.StatusInternalServerError,
			wantErr:    "missing QueueLeaves response",
		},
		{
			desc:       "missing-backend-leaf",
			body:       string(logHeadJSON),
			rpcRsp:     &trillian.QueueLeavesResponse{},
			wantStatus: http.StatusInternalServerError,
			wantErr:    "unexpected QueueLeaves response leaf count",
		},
		{
			desc: "invalid-backend-leaf",
			body: string(logHeadJSON),
			rpcRsp: &trillian.QueueLeavesResponse{
				QueuedLeaves: []*trillian.QueuedLogLeaf{
					{
						Leaf: &trillian.LogLeaf{
							LeafValue:        []byte{0x01, 0x02},
							LeafIndex:        1,
							LeafIdentityHash: leafIdentityHash[:],
						},
						Status: status.New(codes.OK, "OK").Proto(),
					},
				},
			},
			wantStatus: http.StatusInternalServerError,
			wantErr:    "failed to reconstruct TimestampedEntry",
		},
		{
			desc: "backend-leaf-trailing-data",
			body: string(logHeadJSON),
			rpcRsp: &trillian.QueueLeavesResponse{
				QueuedLeaves: []*trillian.QueuedLogLeaf{
					{
						Leaf: &trillian.LogLeaf{
							LeafValue:        append(leafValueData, 0xff),
							LeafIndex:        1,
							LeafIdentityHash: leafIdentityHash[:],
						},
						Status: status.New(codes.OK, "OK").Proto(),
					},
				},
			},
			wantStatus: http.StatusInternalServerError,
			wantErr:    "extra data (1 bytes) on reconstructing TimestampedEntry",
		},
		{
			desc:   "signer-failure",
			body:   string(logHeadJSON),
			signer: &testSigner{err: errors.New("signerfails")},
			rpcRsp: &trillian.QueueLeavesResponse{
				QueuedLeaves: []*trillian.QueuedLogLeaf{
					{
						Leaf: &trillian.LogLeaf{
							LeafValue:        leafValueData,
							LeafIndex:        1,
							LeafIdentityHash: leafIdentityHash[:],
						},
						Status: status.New(codes.OK, "OK").Proto(),
					},
				},
			},
			wantStatus: http.StatusInternalServerError,
			wantErr:    "failed to sign SGT data",
		},
		{
			desc: "valid",
			body: string(logHeadJSON),
			rpcRsp: &trillian.QueueLeavesResponse{
				QueuedLeaves: []*trillian.QueuedLogLeaf{
					{
						Leaf: &trillian.LogLeaf{
							LeafValue:        leafValueData,
							LeafIndex:        1,
							LeafIdentityHash: leafIdentityHash[:],
						},
						Status: status.New(codes.OK, "OK").Proto(),
					},
				},
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			info := setupTest(t, test.signer)
			defer info.mockCtrl.Finish()
			req, err := http.NewRequest("POST", "http://example.com/gossip/v0/add-signed-blob", strings.NewReader(test.body))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			if test.rpcRsp != nil || test.rpcErr != nil || test.forceRPC {
				info.client.EXPECT().QueueLeaves(ctx, gomock.Any()).Return(test.rpcRsp, test.rpcErr)
			}

			gotRsp := httptest.NewRecorder()
			gotStatus, gotErr := addSignedBlob(ctx, info.hi, gotRsp, req)

			if gotStatus != test.wantStatus {
				t.Errorf("addSignedBlob()=%d,_; want %d,_", gotStatus, test.wantStatus)
			}
			if gotErr != nil {
				if len(test.wantErr) == 0 {
					t.Errorf("addSignedBlob()=_,%v; want _,nil", gotErr)
				} else if !strings.Contains(gotErr.Error(), test.wantErr) {
					t.Errorf("addSignedBlob()=_,%v; want _, err containing %q", gotErr, test.wantErr)
				}
				return
			}
			if len(test.wantErr) > 0 {
				t.Errorf("addSignedBlob()=_,nil; want _, err containing %q", test.wantErr)
			}

			var got api.AddSignedBlobResponse
			if err := json.Unmarshal(gotRsp.Body.Bytes(), &got); err != nil {
				t.Fatalf("Failed to unmarshal json response: %s", gotRsp.Body.Bytes())
			}
			if got, want := got.TimestampedEntryData, leafValueData; !reflect.DeepEqual(got, want) {
				t.Errorf("addSignedBlob().TimestampedEntryData=%x; want %x", got, want)
			}
		})
	}

}

func TestGetSTH(t *testing.T) {
	ctx := context.Background()
	req, err := http.NewRequest("GET", "http://example.com/gossip/v0/get-sth", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	tests := []struct {
		desc       string
		rpcRsp     *trillian.GetLatestSignedLogRootResponse
		rpcErr     error
		signer     crypto.Signer
		wantStatus int
		wantErr    string
	}{
		{
			desc:       "backend-failure",
			rpcErr:     errors.New("backendfailure"),
			wantStatus: http.StatusInternalServerError,
			wantErr:    "backendfailure",
		},
		{
			desc:       "backend-nil-root",
			rpcRsp:     &trillian.GetLatestSignedLogRootResponse{},
			wantStatus: http.StatusInternalServerError,
			wantErr:    "no log root returned",
		},
		{
			desc: "backend-invalid-root",
			rpcRsp: &trillian.GetLatestSignedLogRootResponse{
				SignedLogRoot: &trillian.SignedLogRoot{LogRoot: []byte{0x01}},
			},
			wantStatus: http.StatusInternalServerError,
			wantErr:    "failed to unmarshal",
		},
		{
			desc:       "signer-fail",
			rpcRsp:     makeGetSLRRsp(t, 12345, 25, []byte("abcdabcdabcdabcdabcdabcdabcdabcd")),
			signer:     &testSigner{err: errors.New("signerfails")},
			wantStatus: http.StatusInternalServerError,
			wantErr:    "signerfails",
		},
		{
			desc:       "ok",
			rpcRsp:     makeGetSLRRsp(t, 12345000000, 25, []byte("abcdabcdabcdabcdabcdabcdabcdabcd")),
			wantStatus: http.StatusOK,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			info := setupTest(t, test.signer)
			defer info.mockCtrl.Finish()
			rpcReq := &trillian.GetLatestSignedLogRootRequest{LogId: testTreeID}
			info.client.EXPECT().GetLatestSignedLogRoot(ctx, rpcReq).Return(test.rpcRsp, test.rpcErr)

			gotRsp := httptest.NewRecorder()
			gotStatus, gotErr := getSTH(ctx, info.hi, gotRsp, req)

			if gotStatus != test.wantStatus {
				t.Errorf("getSTH()=%d,_; want %d,_", gotStatus, test.wantStatus)
			}
			if gotErr != nil {
				if len(test.wantErr) == 0 {
					t.Errorf("getSTH()=_,%v; want _,nil", gotErr)
				} else if !strings.Contains(gotErr.Error(), test.wantErr) {
					t.Errorf("getSTH()=_,%v; want _, err containing %q", gotErr, test.wantErr)
				}
				return
			}
			if len(test.wantErr) > 0 {
				t.Errorf("getSTH()=_,nil; want _, err containing %q", test.wantErr)
			}

			var rsp api.GetSTHResponse
			if err := json.Unmarshal(gotRsp.Body.Bytes(), &rsp); err != nil {
				t.Fatalf("Failed to unmarshal json response: %s", gotRsp.Body.Bytes())
			}
		})
	}
}

func TestGetSTHConsistency(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		desc          string
		uri           string
		first, second int64
		rpcRsp        *trillian.GetConsistencyProofResponse
		rpcErr        error
		wantStatus    int
		wantErr       string
		want          [][]byte
	}{
		{
			desc:       "no-first",
			uri:        "http:/example.com/gossip/v0/get-sth-consistency?second=2",
			wantStatus: http.StatusBadRequest,
			wantErr:    "parameter 'first' is required",
		},
		{
			desc:       "no-second",
			uri:        "http:/example.com/gossip/v0/get-sth-consistency?first=1",
			wantStatus: http.StatusBadRequest,
			wantErr:    "parameter 'second' is required",
		},
		{
			desc:       "first-malformed",
			uri:        "http:/example.com/gossip/v0/get-sth-consistency?first=a&second=2",
			wantStatus: http.StatusBadRequest,
			wantErr:    "parameter 'first' is malformed",
		},
		{
			desc:       "second-malformed",
			uri:        "http:/example.com/gossip/v0/get-sth-consistency?first=1&second=a",
			wantStatus: http.StatusBadRequest,
			wantErr:    "parameter 'second' is malformed",
		},
		{
			desc:       "first-negative",
			uri:        "http:/example.com/gossip/v0/get-sth-consistency?first=-1&second=2",
			wantStatus: http.StatusBadRequest,
			wantErr:    "cannot be <0",
		},
		{
			desc:       "second-negative",
			uri:        "http:/example.com/gossip/v0/get-sth-consistency?first=1&second=-2",
			wantStatus: http.StatusBadRequest,
			wantErr:    "cannot be <0",
		},
		{
			desc:       "range-inverted",
			uri:        "http:/example.com/gossip/v0/get-sth-consistency?first=2&second=1",
			wantStatus: http.StatusBadRequest,
			wantErr:    "invalid first, second",
		},
		{
			desc:       "first-zero",
			uri:        "http:/example.com/gossip/v0/get-sth-consistency?first=0&second=2",
			first:      0,
			second:     2,
			wantStatus: http.StatusOK,
			want:       [][]byte{}, // empty proof for anything starting from zero
		},
		{
			desc:       "backend-failure",
			uri:        "http:/example.com/gossip/v0/get-sth-consistency?first=1&second=2",
			first:      1,
			second:     2,
			rpcErr:     errors.New("backendfailure"),
			wantStatus: http.StatusInternalServerError,
			wantErr:    "backendfailure",
		},
		{
			desc:   "malformed-slr",
			uri:    "http:/example.com/gossip/v0/get-sth-consistency?first=1&second=2",
			first:  1,
			second: 2,
			rpcRsp: &trillian.GetConsistencyProofResponse{
				Proof: &trillian.Proof{
					LeafIndex: 1,
					Hashes:    [][]byte{[]byte("01234567890123456789012345678901")},
				},
				SignedLogRoot: &trillian.SignedLogRoot{
					LogRoot: []byte{0xFF},
				},
			},
			wantStatus: http.StatusInternalServerError,
			wantErr:    "failed to unmarshal log root",
		},
		{
			desc:   "tree-too-small",
			uri:    "http:/example.com/gossip/v0/get-sth-consistency?first=1001&second=1002",
			first:  1001,
			second: 1002,
			rpcRsp: &trillian.GetConsistencyProofResponse{
				Proof: &trillian.Proof{
					LeafIndex: 1,
					Hashes:    [][]byte{[]byte("01234567890123456789012345678901")},
				},
				SignedLogRoot: makeSLR(t, 12345000000, 25, []byte("abcdabcdabcdabcdabcdabcdabcdabcd")),
			},
			wantStatus: http.StatusBadRequest,
			wantErr:    "need tree size",
		},
		{
			desc:   "invalid-hash-length",
			uri:    "http:/example.com/gossip/v0/get-sth-consistency?first=1&second=2",
			first:  1,
			second: 2,
			rpcRsp: &trillian.GetConsistencyProofResponse{
				Proof: &trillian.Proof{
					LeafIndex: 1,
					Hashes:    [][]byte{{0xde, 0xad}},
				},
				SignedLogRoot: makeSLR(t, 12345000000, 25, []byte("abcdabcdabcdabcdabcdabcdabcdabcd")),
			},
			wantStatus: http.StatusInternalServerError,
			wantErr:    "backend returned invalid proof",
		},
		{
			desc:   "empty-proof",
			uri:    "http:/example.com/gossip/v0/get-sth-consistency?first=2&second=2",
			first:  2,
			second: 2,
			rpcRsp: &trillian.GetConsistencyProofResponse{
				Proof: &trillian.Proof{
					LeafIndex: 1,
				},
				SignedLogRoot: makeSLR(t, 12345000000, 25, []byte("abcdabcdabcdabcdabcdabcdabcdabcd")),
			},
			wantStatus: http.StatusOK,
			want:       [][]byte{},
		},
		{
			desc:   "ok",
			uri:    "http:/example.com/gossip/v0/get-sth-consistency?first=1&second=2",
			first:  1,
			second: 2,
			rpcRsp: &trillian.GetConsistencyProofResponse{
				Proof: &trillian.Proof{
					LeafIndex: 1,
					Hashes:    [][]byte{[]byte("01234567890123456789012345678901")},
				},
				SignedLogRoot: makeSLR(t, 12345000000, 25, []byte("abcdabcdabcdabcdabcdabcdabcdabcd")),
			},
			wantStatus: http.StatusOK,
			want:       [][]byte{[]byte("01234567890123456789012345678901")},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			req, err := http.NewRequest("GET", test.uri, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			info := setupTest(t, nil)
			defer info.mockCtrl.Finish()
			if test.rpcRsp != nil || test.rpcErr != nil {
				rpcReq := &trillian.GetConsistencyProofRequest{LogId: testTreeID, FirstTreeSize: test.first, SecondTreeSize: test.second}
				info.client.EXPECT().GetConsistencyProof(ctx, rpcReq).Return(test.rpcRsp, test.rpcErr)
			}

			gotRsp := httptest.NewRecorder()
			gotStatus, gotErr := getSTHConsistency(ctx, info.hi, gotRsp, req)

			if gotStatus != test.wantStatus {
				t.Errorf("getSTHConsistency(%d,%d)=%d,_; want %d,_", test.first, test.second, gotStatus, test.wantStatus)
			}
			if gotErr != nil {
				if len(test.wantErr) == 0 {
					t.Errorf("getSTHConsistency(%d,%d)=_,%v; want _,nil", test.first, test.second, gotErr)
				} else if !strings.Contains(gotErr.Error(), test.wantErr) {
					t.Errorf("getSTHConsistency(%d,%d)=_,%v; want _, err containing %q", test.first, test.second, gotErr, test.wantErr)
				}
				return
			}
			if len(test.wantErr) > 0 {
				t.Errorf("getSTHConsistency(%d,%d)=_,nil; want _, err containing %q", test.first, test.second, test.wantErr)
			}

			var rsp api.GetSTHConsistencyResponse
			if err := json.Unmarshal(gotRsp.Body.Bytes(), &rsp); err != nil {
				t.Fatalf("Failed to unmarshal json response: %s", gotRsp.Body.Bytes())
			}
			if got := rsp.Consistency; !reflect.DeepEqual(got, test.want) {
				t.Errorf("getSTHConsistency(%d,%d)=%x; want %x", test.first, test.second, got, test.want)
			}
		})
	}
}

func TestGetProofByHash(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		desc       string
		uri        string
		hash       []byte
		treeSize   int64
		rpcRsp     *trillian.GetInclusionProofByHashResponse
		rpcErr     error
		wantStatus int
		wantErr    string
		want       api.GetProofByHashResponse
	}{
		{
			desc:       "no-hash",
			uri:        "http:/example.com/gossip/v0/get-proof-by-hash?tree_size=2",
			wantStatus: http.StatusBadRequest,
			wantErr:    "missing/empty hash param",
		},
		{
			desc:       "invalid-hash",
			uri:        "http:/example.com/gossip/v0/get-proof-by-hash?hash=A&tree_size=10",
			wantStatus: http.StatusBadRequest,
			wantErr:    "invalid base64 hash",
		},
		{
			desc:       "size-malformed",
			uri:        "http:/example.com/gossip/v0/get-proof-by-hash?hash=AAAA&tree_size=a",
			wantStatus: http.StatusBadRequest,
			wantErr:    "missing or invalid tree_size",
		},
		{
			desc:       "size-zero",
			uri:        "http:/example.com/gossip/v0/get-proof-by-hash?hash=AAAA&tree_size=0",
			wantStatus: http.StatusBadRequest,
			wantErr:    "missing or invalid tree_size",
		},
		{
			desc:       "size-negative",
			uri:        "http:/example.com/gossip/v0/get-proof-by-hash?hash=AAAA&tree_size=-1",
			wantStatus: http.StatusBadRequest,
			wantErr:    "missing or invalid tree_size",
		},
		{
			desc:       "backend-failure",
			uri:        "http:/example.com/gossip/v0/get-proof-by-hash?hash=AAAA&tree_size=2",
			hash:       []byte{0x00, 0x00, 0x00},
			treeSize:   2,
			rpcErr:     errors.New("backendfailure"),
			wantStatus: http.StatusInternalServerError,
			wantErr:    "backendfailure",
		},
		{
			desc:     "malformed-slr",
			uri:      "http:/example.com/gossip/v0/get-proof-by-hash?hash=AAAA&tree_size=2",
			hash:     []byte{0x00, 0x00, 0x00},
			treeSize: 2,
			rpcRsp: &trillian.GetInclusionProofByHashResponse{
				Proof: []*trillian.Proof{
					{
						LeafIndex: 1,
						Hashes:    [][]byte{[]byte("01234567890123456789012345678901")},
					},
				},
				SignedLogRoot: &trillian.SignedLogRoot{
					LogRoot: []byte{0xFF},
				},
			},
			wantStatus: http.StatusInternalServerError,
			wantErr:    "failed to unmarshal log root",
		},
		{
			desc:     "tree-too-small",
			uri:      "http:/example.com/gossip/v0/get-proof-by-hash?hash=AAAA&tree_size=1002",
			hash:     []byte{0x00, 0x00, 0x00},
			treeSize: 1002,
			rpcRsp: &trillian.GetInclusionProofByHashResponse{
				Proof: []*trillian.Proof{
					{
						LeafIndex: 1,
						Hashes:    [][]byte{[]byte("01234567890123456789012345678901")},
					},
				},
				SignedLogRoot: makeSLR(t, 12345000000, 25, []byte("abcdabcdabcdabcdabcdabcdabcdabcd")),
			},
			wantStatus: http.StatusNotFound,
			wantErr:    "log returned tree size",
		},
		{
			desc:     "invalid-hash-length",
			uri:      "http:/example.com/gossip/v0/get-proof-by-hash?hash=AAAA&tree_size=2",
			hash:     []byte{0x00, 0x00, 0x00},
			treeSize: 2,
			rpcRsp: &trillian.GetInclusionProofByHashResponse{
				Proof: []*trillian.Proof{
					{
						LeafIndex: 1,
						Hashes:    [][]byte{{0xde, 0xad}},
					},
				},
				SignedLogRoot: makeSLR(t, 12345000000, 25, []byte("abcdabcdabcdabcdabcdabcdabcdabcd")),
			},
			wantStatus: http.StatusInternalServerError,
			wantErr:    "backend returned invalid proof",
		},
		{
			desc:     "missing-proof",
			uri:      "http:/example.com/gossip/v0/get-proof-by-hash?hash=AAAA&tree_size=2",
			hash:     []byte{0x00, 0x00, 0x00},
			treeSize: 2,
			rpcRsp: &trillian.GetInclusionProofByHashResponse{
				SignedLogRoot: makeSLR(t, 12345000000, 25, []byte("abcdabcdabcdabcdabcdabcdabcdabcd")),
			},
			wantStatus: http.StatusNotFound,
			wantErr:    "backend did not return a proof",
		},
		{
			desc:     "ok",
			uri:      "http:/example.com/gossip/v0/get-proof-by-hash?hash=AAAA&tree_size=2",
			hash:     []byte{0x00, 0x00, 0x00},
			treeSize: 2,
			rpcRsp: &trillian.GetInclusionProofByHashResponse{
				Proof: []*trillian.Proof{
					{
						LeafIndex: 1,
						Hashes:    [][]byte{[]byte("01234567890123456789012345678901")},
					},
				},
				SignedLogRoot: makeSLR(t, 12345000000, 25, []byte("abcdabcdabcdabcdabcdabcdabcdabcd")),
			},
			wantStatus: http.StatusOK,
			want: api.GetProofByHashResponse{
				LeafIndex: 1,
				AuditPath: [][]byte{[]byte("01234567890123456789012345678901")},
			},
		},
		{
			desc:     "empty-proof",
			uri:      "http:/example.com/gossip/v0/get-proof-by-hash?hash=AAAA&tree_size=2",
			hash:     []byte{0x00, 0x00, 0x00},
			treeSize: 2,
			rpcRsp: &trillian.GetInclusionProofByHashResponse{
				Proof: []*trillian.Proof{
					{LeafIndex: 1},
				},
				SignedLogRoot: makeSLR(t, 12345000000, 25, []byte("abcdabcdabcdabcdabcdabcdabcdabcd")),
			},
			wantStatus: http.StatusOK,
			want: api.GetProofByHashResponse{
				LeafIndex: 1,
				AuditPath: [][]byte{},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			req, err := http.NewRequest("GET", test.uri, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			info := setupTest(t, nil)
			defer info.mockCtrl.Finish()
			if test.rpcRsp != nil || test.rpcErr != nil {
				rpcReq := &trillian.GetInclusionProofByHashRequest{LogId: testTreeID, LeafHash: test.hash, TreeSize: test.treeSize, OrderBySequence: true}
				info.client.EXPECT().GetInclusionProofByHash(ctx, rpcReq).Return(test.rpcRsp, test.rpcErr)
			}

			gotRsp := httptest.NewRecorder()
			gotStatus, gotErr := getProofByHash(ctx, info.hi, gotRsp, req)

			if gotStatus != test.wantStatus {
				t.Errorf("getProofByHash(%x,%d)=%d,_; want %d,_", test.hash, test.treeSize, gotStatus, test.wantStatus)
			}
			if gotErr != nil {
				if len(test.wantErr) == 0 {
					t.Errorf("getProofByHash(%x,%d)=_,%v; want _,nil", test.hash, test.treeSize, gotErr)
				} else if !strings.Contains(gotErr.Error(), test.wantErr) {
					t.Errorf("getProofByHash(%x,%d)=_,%v; want _, err containing %q", test.hash, test.treeSize, gotErr, test.wantErr)
				}
				return
			}
			if len(test.wantErr) > 0 {
				t.Errorf("getProofByHash(%x,%d)=_,nil; want _, err containing %q", test.hash, test.treeSize, test.wantErr)
			}

			var got api.GetProofByHashResponse
			if err := json.Unmarshal(gotRsp.Body.Bytes(), &got); err != nil {
				t.Fatalf("Failed to unmarshal json response: %s", gotRsp.Body.Bytes())
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("getProofByHash(%x,%d)=%x; want %x", test.hash, test.treeSize, got, test.want)
			}
		})
	}
}

func TestGetEntries(t *testing.T) {
	ctx := context.Background()
	entry := api.TimestampedEntry{
		SourceID:        []byte("http://the-log.com"),
		BlobData:        []byte{0x01, 0x02},
		SourceSignature: []byte{0x03, 0x04},
		HubTimestamp:    0x12345678,
	}
	entryData, err := tls.Marshal(entry)
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}

	tests := []struct {
		desc         string
		uri          string
		start, count int64
		rpcRsp       *trillian.GetLeavesByRangeResponse
		rpcErr       error
		wantStatus   int
		wantErr      string
		want         [][]byte
	}{
		{
			desc:       "no-start",
			uri:        "http:/example.com/gossip/v0/get-entries?end=2",
			start:      1,
			count:      2,
			wantStatus: http.StatusBadRequest,
			wantErr:    "bad start value",
		},
		{
			desc:       "no-end",
			uri:        "http:/example.com/gossip/v0/get-entries?start=1",
			wantStatus: http.StatusBadRequest,
			wantErr:    "bad end value",
		},
		{
			desc:       "start-malformed",
			uri:        "http:/example.com/gossip/v0/get-entries?start=a&end=2",
			wantStatus: http.StatusBadRequest,
			wantErr:    "bad start value",
		},
		{
			desc:       "end-malformed",
			uri:        "http:/example.com/gossip/v0/get-entries?start=1&end=a",
			wantStatus: http.StatusBadRequest,
			wantErr:    "bad end value",
		},
		{
			desc:       "start-negative",
			uri:        "http:/example.com/gossip/v0/get-entries?start=-1&end=2",
			wantStatus: http.StatusBadRequest,
			wantErr:    "parameters must be >= 0",
		},
		{
			desc:       "end-negative",
			uri:        "http:/example.com/gossip/v0/get-entries?start=1&end=-2",
			wantStatus: http.StatusBadRequest,
			wantErr:    "parameters must be >= 0",
		},
		{
			desc:       "range-inverted",
			uri:        "http:/example.com/gossip/v0/get-entries?start=2&end=1",
			wantStatus: http.StatusBadRequest,
			wantErr:    "is not a valid range",
		},
		{
			desc:       "backend-failure",
			uri:        "http:/example.com/gossip/v0/get-entries?start=1&end=2",
			start:      1,
			count:      2,
			rpcErr:     errors.New("backendfailure"),
			wantStatus: http.StatusInternalServerError,
			wantErr:    "backendfailure",
		},
		{
			desc:  "malformed-slr",
			uri:   "http:/example.com/gossip/v0/get-entries?start=1&end=2",
			start: 1,
			count: 2,
			rpcRsp: &trillian.GetLeavesByRangeResponse{
				SignedLogRoot: &trillian.SignedLogRoot{
					LogRoot: []byte{0xFF},
				},
			},
			wantStatus: http.StatusInternalServerError,
			wantErr:    "failed to unmarshal log root",
		},
		{
			desc:  "tree-too-small",
			uri:   "http:/example.com/gossip/v0/get-entries?start=1001&end=1002",
			start: 1001,
			count: 2,
			rpcRsp: &trillian.GetLeavesByRangeResponse{
				SignedLogRoot: makeSLR(t, 12345000000, 25, []byte("abcdabcdabcdabcdabcdabcdabcdabcd")),
			},
			wantStatus: http.StatusBadRequest,
			wantErr:    "need tree size",
		},
		{
			desc:  "truncate-range",
			uri:   "http:/example.com/gossip/v0/get-entries?start=1&end=20000",
			start: 1,
			count: 1000,
			rpcRsp: &trillian.GetLeavesByRangeResponse{
				Leaves: []*trillian.LogLeaf{
					{LeafIndex: 1, LeafValue: entryData},
					{LeafIndex: 2, LeafValue: entryData},
				},
				SignedLogRoot: makeSLR(t, 12345000000, 25, []byte("abcdabcdabcdabcdabcdabcdabcdabcd")),
			},
			wantStatus: http.StatusOK,
			want:       [][]byte{[]byte("01234567890123456789012345678901")},
		},
		{
			desc:  "more-leaves-than-requested",
			uri:   "http:/example.com/gossip/v0/get-entries?start=1&end=2",
			start: 1,
			count: 2,
			rpcRsp: &trillian.GetLeavesByRangeResponse{
				Leaves: []*trillian.LogLeaf{
					{LeafIndex: 1, LeafValue: entryData},
					{LeafIndex: 2, LeafValue: entryData},
					{LeafIndex: 3, LeafValue: entryData},
				},
				SignedLogRoot: makeSLR(t, 12345000000, 25, []byte("abcdabcdabcdabcdabcdabcdabcdabcd")),
			},
			wantStatus: http.StatusInternalServerError,
			wantErr:    "too many leaves",
		},
		{
			desc:  "wrong-index",
			uri:   "http:/example.com/gossip/v0/get-entries?start=1&end=2",
			start: 1,
			count: 2,
			rpcRsp: &trillian.GetLeavesByRangeResponse{
				Leaves: []*trillian.LogLeaf{
					{LeafIndex: 1, LeafValue: entryData},
					{LeafIndex: 1, LeafValue: entryData},
				},
				SignedLogRoot: makeSLR(t, 12345000000, 25, []byte("abcdabcdabcdabcdabcdabcdabcdabcd")),
			},
			wantStatus: http.StatusInternalServerError,
			wantErr:    "unexpected leaf index",
		},
		{
			desc:  "leaf-trailing-data",
			uri:   "http:/example.com/gossip/v0/get-entries?start=1&end=2",
			start: 1,
			count: 2,
			rpcRsp: &trillian.GetLeavesByRangeResponse{
				Leaves: []*trillian.LogLeaf{
					{LeafIndex: 1, LeafValue: entryData},
					{LeafIndex: 2, LeafValue: append(entryData, 0xff)},
				},
				SignedLogRoot: makeSLR(t, 12345000000, 25, []byte("abcdabcdabcdabcdabcdabcdabcdabcd")),
			},
			wantStatus: http.StatusInternalServerError,
			wantErr:    "Trailing data after Merkle leaf",
		},
		{
			desc:  "leaf-malformed",
			uri:   "http:/example.com/gossip/v0/get-entries?start=1&end=2",
			start: 1,
			count: 2,
			rpcRsp: &trillian.GetLeavesByRangeResponse{
				Leaves: []*trillian.LogLeaf{
					{LeafIndex: 1, LeafValue: entryData},
					{LeafIndex: 2, LeafValue: []byte{0xff}},
				},
				SignedLogRoot: makeSLR(t, 12345000000, 25, []byte("abcdabcdabcdabcdabcdabcdabcdabcd")),
			},
			wantStatus: http.StatusInternalServerError,
			wantErr:    "Failed to deserialize Merkle leaf",
		},
		{
			desc:  "ok",
			uri:   "http:/example.com/gossip/v0/get-entries?start=1&end=2",
			start: 1,
			count: 2,
			rpcRsp: &trillian.GetLeavesByRangeResponse{
				Leaves: []*trillian.LogLeaf{
					{LeafIndex: 1, LeafValue: entryData},
					{LeafIndex: 2, LeafValue: entryData},
				},
				SignedLogRoot: makeSLR(t, 12345000000, 25, []byte("abcdabcdabcdabcdabcdabcdabcdabcd")),
			},
			wantStatus: http.StatusOK,
			want:       [][]byte{[]byte("01234567890123456789012345678901")},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			req, err := http.NewRequest("GET", test.uri, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			info := setupTest(t, nil)
			defer info.mockCtrl.Finish()
			if test.rpcRsp != nil || test.rpcErr != nil {
				rpcReq := &trillian.GetLeavesByRangeRequest{LogId: testTreeID, StartIndex: test.start, Count: test.count}
				info.client.EXPECT().GetLeavesByRange(ctx, rpcReq).Return(test.rpcRsp, test.rpcErr)
			}

			gotRsp := httptest.NewRecorder()
			gotStatus, gotErr := getEntries(ctx, info.hi, gotRsp, req)

			if gotStatus != test.wantStatus {
				t.Errorf("getEntries(%d,+%d)=%d,_; want %d,_", test.start, test.count, gotStatus, test.wantStatus)
			}
			if gotErr != nil {
				if len(test.wantErr) == 0 {
					t.Errorf("getEntries(%d,+%d)=_,%v; want _,nil", test.start, test.count, gotErr)
				} else if !strings.Contains(gotErr.Error(), test.wantErr) {
					t.Errorf("getEntries(%d,+%d)=_,%v; want _, err containing %q", test.start, test.count, gotErr, test.wantErr)
				}
				return
			}
			if len(test.wantErr) > 0 {
				t.Errorf("getEntries(%d,+%d)=_,nil; want _, err containing %q", test.start, test.count, test.wantErr)
			}

			var rsp api.GetEntriesResponse
			if err := json.Unmarshal(gotRsp.Body.Bytes(), &rsp); err != nil {
				t.Fatalf("Failed to unmarshal json response: %s", gotRsp.Body.Bytes())
			}
			if got, want := rsp.Entries, [][]byte{entryData, entryData}; !reflect.DeepEqual(got, want) {
				t.Errorf("getEntries(%d,+%d)=%x; want %x", test.start, test.count, got, want)
			}
		})
	}
}

func TestGetSourceKeys(t *testing.T) {
	ctx := context.Background()
	req, err := http.NewRequest("GET", "http://example.com/gossip/v0/get-source-keys", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	info := setupTest(t, nil)
	defer info.mockCtrl.Finish()

	gotRsp := httptest.NewRecorder()
	gotStatus, gotErr := getSourceKeys(ctx, info.hi, gotRsp, req)
	if wantStatus := http.StatusOK; gotStatus != wantStatus {
		t.Errorf("getSourceKeys()=%d,_; want %d,_", gotStatus, wantStatus)
	}
	if gotErr != nil {
		t.Fatalf("getSourceKeys()=_,%v; want _,nil", gotErr)
	}

	var rsp api.GetSourceKeysResponse
	if err := json.Unmarshal(gotRsp.Body.Bytes(), &rsp); err != nil {
		t.Fatalf("Failed to unmarshal json response: %s", gotRsp.Body.Bytes())
	}
	if got, want := sourceSet(t, rsp.Entries), sourceSet(t, testSources); !reflect.DeepEqual(got, want) {
		t.Errorf("getSourceKeys()=%+v, want %+v", got, want)
	}
}

// sourceSet converts a slice of SourceKey objects into a set of strings
// to allow orderless comparison.
func sourceSet(t *testing.T, srcs []*api.SourceKey) map[string]bool {
	t.Helper()
	result := make(map[string]bool)
	for _, src := range srcs {
		data, err := json.Marshal(src)
		if err != nil {
			t.Fatalf("Failed to JSON marshal entry: %v", err)
		}
		result[string(data)] = true
	}
	return result
}

func TestGetLatestForSource(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		desc       string
		uri        string
		entry      *api.TimestampedEntry
		wantStatus int
		wantErr    string
	}{
		{
			desc:       "no-src",
			uri:        "http:/example.com/gossip/v0/get-latest-for-src",
			wantStatus: http.StatusNotFound,
			wantErr:    "unknown source",
		},
		{
			desc:       "unknown-src",
			uri:        "http:/example.com/gossip/v0/get-latest-for-src?src_id=bogus",
			wantStatus: http.StatusNotFound,
			wantErr:    "unknown source",
		},
		{
			desc:       "no-latest",
			uri:        fmt.Sprintf("http:/example.com/gossip/v0/get-latest-for-src?src_id=%s", testSourceID),
			wantStatus: http.StatusNoContent,
			wantErr:    "no latest entry",
		},
		{
			desc: "ok",
			uri:  fmt.Sprintf("http:/example.com/gossip/v0/get-latest-for-src?src_id=%s", testSourceID),
			entry: &api.TimestampedEntry{
				SourceID:        []byte(testSourceID),
				BlobData:        []byte{0x01, 0x02},
				SourceSignature: []byte{0x03, 0x04},
				HubTimestamp:    0x1000000,
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			req, err := http.NewRequest("GET", test.uri, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			info := setupTest(t, nil)
			defer info.mockCtrl.Finish()
			info.hi.setLatestEntry(testSourceID, test.entry)

			gotRsp := httptest.NewRecorder()
			gotStatus, gotErr := getLatestForSource(ctx, info.hi, gotRsp, req)

			if gotStatus != test.wantStatus {
				t.Errorf("getLatestForSource()=%d,_; want %d,_", gotStatus, test.wantStatus)
			}
			if gotErr != nil {
				if len(test.wantErr) == 0 {
					t.Errorf("getLatestForSource()=_,%v; want _,nil", gotErr)
				} else if !strings.Contains(gotErr.Error(), test.wantErr) {
					t.Errorf("getLatestForSource()=_,%v; want _, err containing %q", gotErr, test.wantErr)
				}
				return
			}
			if len(test.wantErr) > 0 {
				t.Errorf("getLatestForSource()=_,nil; want _, err containing %q", test.wantErr)
			}

			var rsp api.GetLatestForSourceResponse
			if err := json.Unmarshal(gotRsp.Body.Bytes(), &rsp); err != nil {
				t.Fatalf("Failed to unmarshal json response: %s", gotRsp.Body.Bytes())
			}
			want, _ := tls.Marshal(*test.entry)
			if got := rsp.Entry; !bytes.Equal(got, want) {
				t.Errorf("getLatestForSource()=%x; want %x", got, want)
			}
		})
	}
}

func TestToHTTPStatus(t *testing.T) {
	tests := []struct {
		err    error
		mapper func(error) (int, bool)
		want   int
	}{
		{err: nil, want: http.StatusOK},
		{err: errors.New("test"), want: http.StatusInternalServerError},
		{err: status.Error(codes.OK, "test"), want: http.StatusOK},
		{err: status.Error(codes.Canceled, "test"), want: http.StatusRequestTimeout},
		{err: status.Error(codes.DeadlineExceeded, "test"), want: http.StatusRequestTimeout},
		{err: status.Error(codes.InvalidArgument, "test"), want: http.StatusBadRequest},
		{err: status.Error(codes.OutOfRange, "test"), want: http.StatusBadRequest},
		{err: status.Error(codes.AlreadyExists, "test"), want: http.StatusBadRequest},
		{err: status.Error(codes.NotFound, "test"), want: http.StatusNotFound},
		{err: status.Error(codes.PermissionDenied, "test"), want: http.StatusForbidden},
		{err: status.Error(codes.ResourceExhausted, "test"), want: http.StatusForbidden},
		{err: status.Error(codes.Unauthenticated, "test"), want: http.StatusUnauthorized},
		{err: status.Error(codes.FailedPrecondition, "test"), want: http.StatusPreconditionFailed},
		{err: status.Error(codes.Aborted, "test"), want: http.StatusConflict},
		{err: status.Error(codes.Unimplemented, "test"), want: http.StatusNotImplemented},
		{err: status.Error(codes.Unavailable, "test"), want: http.StatusServiceUnavailable},
		{err: status.Error(9999, "test"), want: http.StatusInternalServerError},
		{err: errors.New("test"), mapper: func(error) (int, bool) { return 123, true }, want: 123},
		{err: errors.New("test"), mapper: func(error) (int, bool) { return 123, false }, want: http.StatusInternalServerError},
		{err: status.Error(codes.Canceled, "test"), mapper: func(error) (int, bool) { return 123, false }, want: http.StatusRequestTimeout},
	}
	for _, test := range tests {
		hi := hubInfo{opts: InstanceOptions{ErrorMapper: test.mapper}}
		got := hi.toHTTPStatus(test.err)
		if got != test.want {
			t.Errorf("toHTTPStatus(%T %v)=%d, want %d", test.err, test.err, got, test.want)
		}
	}
}

// testSigner implements crypto.Signer and always returns a fixed signature and error.
type testSigner struct {
	publicKey crypto.PublicKey
	signature []byte
	err       error
}

// Public returns the public key associated with the signer that this stub is based on.
func (s *testSigner) Public() crypto.PublicKey {
	return s.publicKey
}

// Sign returns the will return the signature or error that the signerStub was created to provide.
func (s *testSigner) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	return s.signature, s.err
}
