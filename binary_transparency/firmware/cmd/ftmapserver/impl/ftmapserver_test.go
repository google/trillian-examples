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

package impl

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	gomock "github.com/golang/mock/gomock"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian/experimental/batchmap"
	"github.com/google/trillian/types"
	"github.com/gorilla/mux"
)

func TestRoot(t *testing.T) {
	for _, test := range []struct {
		desc     string
		rev      int
		logroot  types.LogRootV1
		count    int64
		rootHash []byte
		wantBody string
	}{
		{
			desc: "valid 1",
			rev:  42,
			logroot: types.LogRootV1{
				TreeSize:       42,
				RootHash:       []byte{0x12, 0x34},
				Revision:       2,
				TimestampNanos: 12345,
			},
			count:    111,
			rootHash: []byte{0x34, 0x12},
			// LogCheckpoint below is base64 of:
			// Firmware Transparency Log v0
			// 42
			// EjQ=
			// 12345
			wantBody: `{"LogCheckpoint":"RmlybXdhcmUgVHJhbnNwYXJlbmN5IExvZyB2MAo0MgpFalE9CjEyMzQ1Cg==","LogSize":111,"RootHash":"NBI=","Revision":42}`,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mmr := NewMockMapReader(ctrl)
			server := Server{db: mmr}

			mmr.EXPECT().LatestRevision().Return(test.rev, test.logroot, test.count, nil /* err */)
			mmr.EXPECT().Tile(test.rev, []byte{}).Return(&batchmap.Tile{RootHash: test.rootHash}, nil /* err */)

			ts := httptest.NewServer(http.HandlerFunc(server.getCheckpoint))
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

func TestTile(t *testing.T) {
	for _, test := range []struct {
		desc     string
		rev      int
		path     []byte
		leaf     batchmap.TileLeaf
		wantBody string
	}{
		{
			desc:     "root",
			rev:      42,
			path:     []byte{},
			leaf:     batchmap.TileLeaf{Path: []byte{0x01}, Hash: []byte{0x12, 0x34}},
			wantBody: `{"Path":"","Leaves":[{"Path":"AQ==","Hash":"EjQ="}]}`,
		},
		{
			desc:     "deeper",
			rev:      42,
			path:     []byte{0x01},
			leaf:     batchmap.TileLeaf{Path: []byte{0x02}, Hash: []byte{0x12, 0x34}},
			wantBody: `{"Path":"AQ==","Leaves":[{"Path":"Ag==","Hash":"EjQ="}]}`,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mmr := NewMockMapReader(ctrl)
			server := Server{db: mmr}

			mmr.EXPECT().Tile(test.rev, test.path).Return(&batchmap.Tile{
				Path:   test.path,
				Leaves: []*batchmap.TileLeaf{&test.leaf},
			}, nil /* err */)

			r := mux.NewRouter()
			server.RegisterHandlers(r)
			ts := httptest.NewServer(r)
			defer ts.Close()
			url := fmt.Sprintf("%s/%s/in-revision/%d/at-path/%s", ts.URL, api.MapHTTPGetTile, test.rev, base64.URLEncoding.EncodeToString(test.path))

			client := ts.Client()
			resp, err := client.Get(url)
			if err != nil {
				t.Errorf("error response: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("status code not OK: %v (%s)", resp.StatusCode, url)
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

func TestAggregation(t *testing.T) {
	for _, test := range []struct {
		desc       string
		rev        int
		fwLogIndex uint64
		agg        api.AggregatedFirmware
		wantBody   string
	}{
		{
			desc:       "index 0",
			rev:        42,
			fwLogIndex: 0,
			agg:        api.AggregatedFirmware{Index: 0, Good: true},
			wantBody:   `{"Index":0,"Good":true}`,
		},
		{
			desc:       "index 1",
			rev:        101,
			fwLogIndex: 1,
			agg:        api.AggregatedFirmware{Index: 1, Good: false},
			wantBody:   `{"Index":1,"Good":false}`,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mmr := NewMockMapReader(ctrl)
			server := Server{db: mmr}

			mmr.EXPECT().Aggregation(test.rev, test.fwLogIndex).Return(test.agg, nil /* err */)

			r := mux.NewRouter()
			server.RegisterHandlers(r)
			ts := httptest.NewServer(r)
			defer ts.Close()
			url := fmt.Sprintf("%s/%s/in-revision/%d/for-firmware-at-index/%d", ts.URL, api.MapHTTPGetAggregation, test.rev, test.fwLogIndex)

			client := ts.Client()
			resp, err := client.Get(url)
			if err != nil {
				t.Errorf("error response: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("status code not OK: %v (%s)", resp.StatusCode, url)
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
