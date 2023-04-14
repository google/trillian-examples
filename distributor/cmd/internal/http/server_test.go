// Copyright 2023 Google LLC. All Rights Reserved.
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

package http_test

//go:generate mockgen -write_package_comment=false -self_package github.com/google/trillian-examples/distributor/cmd/internal/http_test -package http_test -destination mock_distributor_test.go  github.com/google/trillian-examples/distributor/cmd/internal/http Distributor

import (
	"fmt"
	"io"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian-examples/distributor/cmd/internal/http"
	"github.com/gorilla/mux"

	_ "github.com/mattn/go-sqlite3" // Load drivers for sqlite3
)

func createTestEnv(d http.Distributor) (*httptest.Server, func()) {
	r := mux.NewRouter()
	server := http.NewServer(d)
	server.RegisterHandlers(r)
	ts := httptest.NewServer(r)
	return ts, ts.Close
}

func TestGetByWitness(t *testing.T) {
	testCases := []struct {
		desc           string
		logid          string
		witid          string
		cpReturn       []byte
		wantStatusCode int
	}{
		{
			desc:           "happy path",
			logid:          "thisisalog",
			witid:          "happyhappywitness",
			cpReturn:       []byte{},
			wantStatusCode: 200,
		},
		{
			desc:           "complex but valid witness ID",
			logid:          "thisisalog",
			witid:          "happy.tricky/witness",
			cpReturn:       []byte{},
			wantStatusCode: 200,
		},
		{
			desc:           "empty witness ID",
			logid:          "thisisalog",
			witid:          "",
			cpReturn:       []byte("404 page not found\n"),
			wantStatusCode: 404,
		},
		{
			desc:           "invalid witness ID spaces",
			logid:          "thisisalog",
			witid:          "not legit",
			cpReturn:       []byte("404 page not found\n"),
			wantStatusCode: 404,
		},
		{
			desc:           "invalid witness ID pluses",
			logid:          "thisisalog",
			witid:          "not+legit",
			cpReturn:       []byte("404 page not found\n"),
			wantStatusCode: 404,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			d := NewMockDistributor(ctrl)
			s, close := createTestEnv(d)
			defer close()

			d.EXPECT().GetCheckpointWitness(gomock.Any(), gomock.Eq(tC.logid), gomock.Eq(tC.witid)).Return(tC.cpReturn, nil).AnyTimes()

			c := s.Client()
			resp, err := c.Get(fmt.Sprintf("%s/distributor/v0/logs/%s/byWitness/%s/checkpoint", s.URL, url.PathEscape(tC.logid), url.PathEscape(tC.witid)))
			if err != nil {
				t.Error(err)
			}
			if resp.StatusCode != tC.wantStatusCode {
				t.Errorf("expected %d, got %d", tC.wantStatusCode, resp.StatusCode)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Error(err)
			}
			if !cmp.Equal(body, tC.cpReturn) {
				t.Errorf("expected %q, got %q", string(tC.cpReturn), string(body))
			}
		})
	}
}

func TestGetCheckpointN(t *testing.T) {
	testCases := []struct {
		desc           string
		logid          string
		n              int32
		wantBody       []byte
		wantStatusCode int
	}{
		{
			desc:           "happy path",
			logid:          "thisisalog",
			n:              1,
			wantBody:       []byte{},
			wantStatusCode: 200,
		},
		{
			desc:           "negative N",
			logid:          "thisisalog",
			n:              -1,
			wantBody:       []byte("404 page not found\n"),
			wantStatusCode: 404,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			d := NewMockDistributor(ctrl)
			s, close := createTestEnv(d)
			defer close()

			d.EXPECT().GetCheckpointN(gomock.Any(), gomock.Eq(tC.logid), gomock.Eq(uint32(tC.n))).Return(tC.wantBody, nil).AnyTimes()

			c := s.Client()
			resp, err := c.Get(fmt.Sprintf("%s/distributor/v0/logs/%s/checkpoint.%d", s.URL, url.PathEscape(tC.logid), tC.n))
			if err != nil {
				t.Error(err)
			}
			if resp.StatusCode != tC.wantStatusCode {
				t.Errorf("expected %d, got %d", tC.wantStatusCode, resp.StatusCode)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Error(err)
			}
			if !cmp.Equal(body, tC.wantBody) {
				t.Errorf("expected %q, got %q", string(tC.wantBody), string(body))
			}
		})
	}
}
