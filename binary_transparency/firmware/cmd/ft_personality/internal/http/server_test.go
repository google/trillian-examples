package http

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gomock "github.com/golang/mock/gomock"
	"github.com/google/trillian/types"
	"github.com/gorilla/mux"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
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
			server := NewServer(mt)

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

func TestAddFirmware(t *testing.T) {
	for _, test := range []struct {
		desc        string
		body        string
		trillianErr error
		wantStatus  int
	}{
		{
			desc:       "malformed request",
			body:       "garbage",
			wantStatus: http.StatusBadRequest,
		}, {
			desc:       "valid request",
			body:       `{"Metadata":"eyJEZXZpY2VJRCI6IlRhbGtpZVRvYXN0ZXIiLCJGaXJtd2FyZVJldmlzaW9uIjoxLCJGaXJtd2FyZUltYWdlU0hBNTEyIjoiZGdmVVR4Z0tUVGVMYU5UNk9ncFRXTVJOSlRVOFExM29nUEJvc0lsemVDc2gzSE45bm9pMWdPUUo0eFVRRGJFczZWTDN0bXNSWXYwTTEyTVBuaTJlU1E9PSIsIkV4cGVjdGVkRmlybXdhcmVNZWFzdXJlbWVudCI6IiIsIkJ1aWxkVGltZXN0YW1wIjoiMjAyMC0xMS0wNlQxMToyOTo0N1oifQ==","Signature":"TE9MIQ=="}`,
			wantStatus: http.StatusOK,
		}, {
			desc:        "valid request but trillian failure",
			body:        `{"Metadata":"eyJEZXZpY2VJRCI6IlRhbGtpZVRvYXN0ZXIiLCJGaXJtd2FyZVJldmlzaW9uIjoxLCJGaXJtd2FyZUltYWdlU0hBNTEyIjoiZGdmVVR4Z0tUVGVMYU5UNk9ncFRXTVJOSlRVOFExM29nUEJvc0lsemVDc2gzSE45bm9pMWdPUUo0eFVRRGJFczZWTDN0bXNSWXYwTTEyTVBuaTJlU1E9PSIsIkV4cGVjdGVkRmlybXdhcmVNZWFzdXJlbWVudCI6IiIsIkJ1aWxkVGltZXN0YW1wIjoiMjAyMC0xMS0wNlQxMToyOTo0N1oifQ==","Signature":"TE9MIQ=="}`,
			trillianErr: errors.New("boom"),
			wantStatus:  http.StatusInternalServerError,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mt := NewMockTrillian(ctrl)
			server := NewServer(mt)

			mt.EXPECT().AddFirmwareManifest(gomock.Any(), gomock.Eq([]byte(test.body))).
				Return(test.trillianErr)

			r := mux.NewRouter()
			server.RegisterHandlers(r)
			ts := httptest.NewServer(r)
			defer ts.Close()

			client := ts.Client()
			url := fmt.Sprintf("%s/%s", ts.URL, api.HTTPAddFirmware)
			resp, err := client.Post(url, "application/json", strings.NewReader(test.body))
			if err != nil {
				t.Errorf("error response: %v", err)
			}
			if got, want := resp.StatusCode, test.wantStatus; got != want {
				t.Errorf("status code got != want (%d, %d)", got, want)
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
			server := NewServer(mt)
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
			wantBody:      `{"Value":"bGVhZmRhdGE=","Proof":["cHI=","b28=","ZiE="]}`,
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
			server := NewServer(mt)

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
