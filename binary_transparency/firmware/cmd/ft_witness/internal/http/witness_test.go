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

	"github.com/google/trillian-examples/binary_transparency/firmware/internal/crypto"
	"golang.org/x/mod/sumdb/note"
)

const (
	dummyURL          = "xyz"
	dummyPollInterval = 5
)

func TestGetWitnessCheckpoint(t *testing.T) {
	testSigner, _ := note.NewSigner(crypto.TestFTPersonalityPriv)
	testVerifier, _ := note.NewVerifier(crypto.TestFTPersonalityPub)
	for _, test := range []struct {
		desc     string
		wantBody string
	}{
		{
			desc:     "Successful Witness Checkpoint retrieval",
			wantBody: "Firmware Transparency Log\n1\nEjQ=\n123\n",
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			n := &note.Note{
				Text: test.wantBody,
			}
			ns, err := note.Sign(n, testSigner)
			if err != nil {
				t.Fatalf("Failed to sign checkpoint: %v", err)
			}
			witness, err := NewWitness(FakeStore{ns, true}, dummyURL, testVerifier, dummyPollInterval)
			if err != nil {
				t.Fatalf("error creating witness: %v", err)
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
			got, err := note.Open(body, note.VerifierList(testVerifier))
			if err != nil {
				t.Fatalf("Failed to open returned body: %v :\n%s", err, body)
			}
			if got.Text != test.wantBody {
				t.Errorf("got '%s' want '%s'", got.Text, test.wantBody)
			}
		})
	}
}

func TestFailedWitnessCreation(t *testing.T) {
	testVerifier, _ := note.NewVerifier(crypto.TestFTPersonalityPub)
	for _, test := range []struct {
		desc      string
		wantError string
	}{
		{
			desc:      "Failed Witness Creation",
			wantError: "new witness failed due to storage retrieval: unable to access store",
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			_, err := NewWitness(FakeStore{[]byte{}, false}, dummyURL, testVerifier, dummyPollInterval)
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
	scp         []byte
	storeaccess bool
}

func (f FakeStore) StoreCP(wcp []byte) error {
	if !f.storeaccess {
		return fmt.Errorf("unable to access store")
	}
	f.scp = wcp
	return nil
}

func (f FakeStore) RetrieveCP() ([]byte, error) {
	if !f.storeaccess {
		return nil, fmt.Errorf("unable to access store")
	}
	return f.scp, nil
}
