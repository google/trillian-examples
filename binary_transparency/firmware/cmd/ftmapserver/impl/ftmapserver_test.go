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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	gomock "github.com/golang/mock/gomock"
	"github.com/google/trillian/experimental/batchmap"
)

func TestRoot(t *testing.T) {
	for _, test := range []struct {
		desc     string
		rev      int
		logroot  []byte
		count    int64
		rootHash []byte
		wantBody string
	}{
		{
			desc:     "valid 1",
			rev:      42,
			logroot:  []byte{0x12, 0x34},
			count:    111,
			rootHash: []byte{0x34, 0x12},
			wantBody: `{"LogCheckpoint":"EjQ=","LogSize":111,"RootHash":"NBI=","Revision":42}`,
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
