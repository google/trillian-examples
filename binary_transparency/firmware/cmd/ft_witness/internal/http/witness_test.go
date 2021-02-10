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

package http

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
)

const (
	dummyURL          = "xyz"
	dummyPollInterval = 5
)

func TestGetWitnessCheckpoint(t *testing.T) {
	for _, test := range []struct {
		desc     string
		cp       api.LogCheckpoint
		wantBody string
	}{
		{
			desc:     "Successful Witness Checkpoint retrieval",
			cp:       api.LogCheckpoint{TreeSize: 1, TimestampNanos: 123, RootHash: []byte{0x12, 0x34}},
			wantBody: `{"TreeSize":1,"RootHash":"EjQ=","TimestampNanos":123}`,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {

			witness, err := NewWitness(FakeStore{test.cp, true}, dummyURL, dummyPollInterval)
			if err != nil {
				t.Errorf("error creating witness: %v", err)
			}
			ts := httptest.NewServer(http.HandlerFunc(witness.getCheckpoint))
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

func TestFailedWitnessCreation(t *testing.T) {
	for _, test := range []struct {
		desc      string
		cp        api.LogCheckpoint
		wantError string
	}{
		{
			desc:      "Failed Witness Creation",
			cp:        api.LogCheckpoint{},
			wantError: "new witness failed due to storage retrieval: unable to access store",
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			_, err := NewWitness(FakeStore{test.cp, false}, dummyURL, dummyPollInterval)
			if err == nil {
				t.Errorf("error witness creation happened smoothly: %v", err)
			}
			fmt.Printf("Error received %s", err.Error())
			if err.Error() != test.wantError {
				t.Error("Unexpected error message received", err)
			}
		})
	}
}

type FakeStore struct {
	scp         api.LogCheckpoint
	storeaccess bool
}

func (f FakeStore) StoreCP(wcp api.LogCheckpoint) error {
	if !f.storeaccess {
		return fmt.Errorf("unable to access store")
	}
	f.scp = wcp
	return nil
}

func (f FakeStore) RetrieveCP() (api.LogCheckpoint, error) {
	if !f.storeaccess {
		return api.LogCheckpoint{}, fmt.Errorf("unable to access store")
	}
	return f.scp, nil
}
