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
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/google/trillian-examples/witness/golang/api"
	"github.com/google/trillian-examples/witness/golang/internal/persistence/inmemory"
	"github.com/google/trillian-examples/witness/golang/internal/witness"
	"github.com/gorilla/mux"
	"github.com/transparency-dev/merkle/rfc6962"
	"golang.org/x/mod/sumdb/note"

	_ "github.com/mattn/go-sqlite3" // Load drivers for sqlite3
)

var (
	mPK       = "monkeys+87be2a55+AeK/t7elVrIheVCPxQNYkvKFw/2ahkj6Gm9afBJw6S8q"
	bPK       = "bananas+cf639f13+AaPjhFnPCQnid/Ql32KWhmh+uk72FVRfK+2DLmO3BI3M"
	wSK       = "PRIVATE+KEY+witness+f13a86db+AaLa/dfyBhyo/m0Z7WCi98ENVZWtrP8pxgRNrx7tIWiA"
	mInit     = []byte("Log Checkpoint v0\n5\n41smjBUiAU70EtKlT6lIOIYtRTYxYXsDB+XHfcvu/BE=\n\n— monkeys h74qVe5jWoK8CX/zXrT9X80SyEaiwPb/0p7VW7u+cnXxq5pJYQ6vhxUZ5Ywz9WSD3HIyygccizAg+oMxOe6pRgqqOQE=\n")
	bInit     = []byte("Log Checkpoint v0\n5\n41smjBUiAU70EtKlT6lIOIYtRTYxYXsDB+XHfcvu/BE=\n\n— bananas z2OfE18+NwUjjJBXH7m+fh67bu29p1Jbypr4GFUQohgQgCeuPJZtGTvfR9Pquh2Iebfq+6bhl3G/77lsKiGIea6NAwE=\n")
	mNext     = []byte("Log Checkpoint v0\n8\nV8K9aklZ4EPB+RMOk1/8VsJUdFZR77GDtZUQq84vSbo=\n\n— monkeys h74qVetPycmWeWIySx/cMKcLopNS9h2je2DWe2w7PLRmczqdqinRGPscYklpBQO5Un6B5eUMJDwZprVpJie0lSBNPg8=\n")
	consProof = [][]byte{
		dh("b9e1d62618f7fee8034e4c5010f727ab24d8e4705cb296c374bf2025a87a10d2", 32),
		dh("aac66cd7a79ce4012d80762fe8eec3a77f22d1ca4145c3f4cee022e7efcd599d", 32),
		dh("89d0f753f66a290c483b39cd5e9eafb12021293395fad3d4a2ad053cfbcfdc9e", 32),
		dh("29e40bb79c966f4c6fe96aff6f30acfce5f3e8d84c02215175d6e018a5dee833", 32),
	}
	crInit      = []byte("Log Checkpoint v0\n10\ne/S4liN8tioogdwxlCaXdQBb/5bxM9AWKA0bcZa9TXw=\n\n— monkeys h74qVc07jPk7qD0incL5dTlPr9DYoD64X2mJpvIKutiyeBHSm8TGIey7EpRcxA1ZIqEbw2YA6i1a1V4pDjMX51znZQA=\n")
	crInitRange = [][]byte{
		dh("4e856ea495cf591cefb9eff66b311b2d5ec1d0901b8026909f88a3f126d9db11", 32),
		dh("3a357a5ff22c69641ff59c08ca67ccabdefdf317476501db8cafc73ebb5ff547", 32),
	}
	crNext  = []byte("Log Checkpoint v0\n15\nsrKoB8sjvP1QAt1Ih3nqGHzjvmtRLs/geQdehrUHvqs=\n\n— monkeys h74qVU+3T1AVNo+tAsISWzMwEDag8xOwQLSzxJELzHH9N/4cz6M+VbJ89Ku//gybRPPUFP/8Fcvxpqb6++ZyLk83oAE=\n")
	crProof = [][]byte{
		dh("ef626e0b64023948e57f34674c2574b3078c5af59a2faa095f4948736e8ca52e", 32),
		dh("8f75f7d88d3679ac6dd5a68a81215bfbeafe8c566b93013bbc82e64295628c8b", 32),
		dh("e034fb7af8223063c1c299ed91c11a0bc4cec15afd75e2abe4bb54c14d921ef0", 32),
	}
)

const logOrigin = "Log Checkpoint v0"

type logOpts struct {
	ID         string
	origin     string
	PK         string
	useCompact bool
}

func newWitness(t *testing.T, logs []logOpts) *witness.Witness {
	// Set up Opts for the witness.
	ns, err := note.NewSigner(wSK)
	if err != nil {
		t.Fatalf("couldn't create a witness signer: %v", err)
	}
	h := rfc6962.DefaultHasher
	logMap := make(map[string]witness.LogInfo)
	for _, log := range logs {
		logV, err := note.NewVerifier(log.PK)
		if err != nil {
			t.Fatalf("couldn't create a log verifier: %v", err)
		}
		logInfo := witness.LogInfo{
			Origin:     log.origin,
			SigV:       logV,
			Hasher:     h,
			UseCompact: log.useCompact,
		}
		logMap[log.ID] = logInfo
	}
	opts := witness.Opts{
		Persistence: inmemory.NewPersistence(),
		Signer:      ns,
		KnownLogs:   logMap,
	}
	// Create the witness
	w, err := witness.New(opts)
	if err != nil {
		t.Fatalf("couldn't create witness: %v", err)
	}
	return w
}

// dh is taken from https://github.com/google/trillian/blob/master/merkle/logverifier/log_verifier_test.go.
func dh(h string, expLen int) []byte {
	r, err := hex.DecodeString(h)
	if err != nil {
		panic(err)
	}
	if got := len(r); got != expLen {
		panic(fmt.Sprintf("decode %q: len=%d, want %d", h, got, expLen))
	}
	return r
}

func createTestEnv(w *witness.Witness) (*httptest.Server, func()) {
	r := mux.NewRouter()
	server := NewServer(w)
	server.RegisterHandlers(r)
	ts := httptest.NewServer(r)
	return ts, ts.Close
}

func TestGetLogs(t *testing.T) {
	for _, test := range []struct {
		desc       string
		logIDs     []string
		logPKs     []string
		chkpts     [][]byte
		wantStatus int
		wantBody   []string
	}{
		{
			desc:       "no logs",
			logIDs:     []string{},
			wantStatus: http.StatusOK,
			wantBody:   []string{},
		}, {
			desc:       "one log",
			logIDs:     []string{"monkeys"},
			logPKs:     []string{mPK},
			chkpts:     [][]byte{mInit},
			wantStatus: http.StatusOK,
			wantBody:   []string{"monkeys"},
		}, {
			desc:       "two logs",
			logIDs:     []string{"bananas", "monkeys"},
			logPKs:     []string{bPK, mPK},
			chkpts:     [][]byte{bInit, mInit},
			wantStatus: http.StatusOK,
			wantBody:   []string{"bananas", "monkeys"},
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ctx := context.Background()
			// Set up witness and give it some checkpoints.
			logs := make([]logOpts, len(test.logIDs))
			for i, logID := range test.logIDs {
				logs[i] = logOpts{
					ID:         logID,
					origin:     logOrigin,
					PK:         test.logPKs[i],
					useCompact: false,
				}
			}
			w := newWitness(t, logs)
			for i, logID := range test.logIDs {
				if _, err := w.Update(ctx, logID, test.chkpts[i], nil); err != nil {
					t.Errorf("failed to set checkpoint: %v", err)
				}
			}
			// Now set up the http server.
			ts, tsCloseFn := createTestEnv(w)
			defer tsCloseFn()
			client := ts.Client()
			url := fmt.Sprintf("%s%s", ts.URL, api.HTTPGetLogs)
			resp, err := client.Get(url)
			if err != nil {
				t.Errorf("error response: %v", err)
			}
			if got, want := resp.StatusCode, test.wantStatus; got != want {
				t.Errorf("status code got %d, want %d", got, want)
			}
			if len(test.wantBody) > 0 {
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("failed to read body: %v", err)
				}
				var logs []string
				if err := json.Unmarshal(body, &logs); err != nil {
					t.Fatalf("failed to unmarshal body: %v", err)
				}
				if len(logs) != len(test.wantBody) {
					t.Fatalf("got %d logs, want %d", len(logs), len(test.wantBody))
				}
				sort.Strings(logs)
				for i := range logs {
					if logs[i] != test.wantBody[i] {
						t.Fatalf("got %q, want %q", logs[i], test.wantBody[i])
					}
				}
			}
		})
	}
}

func TestGetChkpt(t *testing.T) {
	for _, test := range []struct {
		desc       string
		setID      string
		setPK      string
		queryID    string
		queryPK    string
		c          []byte
		wantStatus int
	}{
		{
			desc:       "happy path",
			setID:      "monkeys",
			setPK:      mPK,
			queryID:    "monkeys",
			queryPK:    mPK,
			c:          mInit,
			wantStatus: http.StatusOK,
		}, {
			desc:       "other log",
			setID:      "monkeys",
			setPK:      mPK,
			queryID:    "bananas",
			c:          mInit,
			wantStatus: http.StatusNotFound,
		}, {
			desc:       "nothing there",
			setID:      "monkeys",
			setPK:      mPK,
			queryID:    "monkeys",
			c:          nil,
			wantStatus: http.StatusNotFound,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ctx := context.Background()
			// Set up witness.
			w := newWitness(t, []logOpts{{
				ID:         test.setID,
				origin:     logOrigin,
				PK:         test.setPK,
				useCompact: false}})
			// Set a checkpoint for the log if we want to for this test.
			if test.c != nil {
				if _, err := w.Update(ctx, test.setID, test.c, nil); err != nil {
					t.Errorf("failed to set checkpoint: %v", err)
				}
			}
			// Now set up the http server.
			ts, tsCloseFn := createTestEnv(w)
			defer tsCloseFn()
			client := ts.Client()
			chkptQ := fmt.Sprintf(api.HTTPGetCheckpoint, test.queryID)
			url := fmt.Sprintf("%s%s", ts.URL, chkptQ)
			resp, err := client.Get(url)
			if err != nil {
				t.Errorf("error response: %v", err)
			}
			if got, want := resp.StatusCode, test.wantStatus; got != want {
				t.Errorf("status code got %d, want %d", got, want)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	for _, test := range []struct {
		desc       string
		initC      []byte
		initSize   uint64
		useCR      bool
		initCR     [][]byte
		body       api.UpdateRequest
		wantStatus int
		wantCP     []byte
	}{
		{
			desc:       "vanilla consistency happy path",
			initC:      mInit,
			initSize:   5,
			body:       api.UpdateRequest{Checkpoint: mNext, Proof: consProof},
			wantStatus: http.StatusOK,
		}, {
			desc:       "vanilla consistency smaller checkpoint",
			initC:      mInit,
			initSize:   5,
			body:       api.UpdateRequest{Checkpoint: []byte("Log Checkpoint v0\n4\nhashhashhash\n\n— monkeys h74qVTDSRnIt40mjmX32f6bSeEFbU67ZpyBltDJuN4KzBlQRe5/jPDCwXRmyWVj72aebO8u42oZVVKy8hjkDg6R0fAs=\n"), Proof: consProof},
			wantStatus: http.StatusConflict,
			wantCP:     mInit,
		}, {
			desc:     "vanilla consistency garbage proof",
			initC:    mInit,
			initSize: 5,
			body: api.UpdateRequest{Checkpoint: mNext, Proof: [][]byte{
				dh("aaaa", 2),
				dh("bbbb", 2),
				dh("cccc", 2),
				dh("dddd", 2),
			}},
			wantStatus: http.StatusBadRequest,
		}, {
			desc:       "vanilla consistency garbage checkpoint",
			initC:      mInit,
			initSize:   5,
			body:       api.UpdateRequest{Checkpoint: []byte("aaa"), Proof: consProof},
			wantStatus: http.StatusBadRequest,
		}, {
			desc:       "compact range happy path",
			initC:      crInit,
			initSize:   10,
			useCR:      true,
			initCR:     crInitRange,
			body:       api.UpdateRequest{Checkpoint: crNext, Proof: crProof},
			wantStatus: http.StatusOK,
		}, {
			desc:     "compact range garbage proof",
			initC:    crInit,
			initSize: 10,
			body: api.UpdateRequest{Checkpoint: crNext, Proof: [][]byte{
				dh("aaaa", 2),
				dh("bbbb", 2),
				dh("cccc", 2),
				dh("dddd", 2),
			}},
			useCR:      true,
			initCR:     crInitRange,
			wantStatus: http.StatusBadRequest,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ctx := context.Background()
			logID := "monkeys"
			// Set up witness.
			w := newWitness(t, []logOpts{{
				ID:         logID,
				origin:     logOrigin,
				PK:         mPK,
				useCompact: test.useCR}})
			// Set an initial checkpoint for the log.
			if _, err := w.Update(ctx, logID, test.initC, test.initCR); err != nil {
				t.Errorf("failed to set checkpoint: %v", err)
			}
			// Now set up the http server.
			ts, tsCloseFn := createTestEnv(w)
			defer tsCloseFn()
			// Update to a newer checkpoint.
			client := ts.Client()
			reqBody, err := json.Marshal(test.body)
			if err != nil {
				t.Fatalf("couldn't parse request: %v", err)
			}
			url := fmt.Sprintf("%s%s", ts.URL, fmt.Sprintf(api.HTTPUpdate, logID))
			req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(string(reqBody)))
			if err != nil {
				t.Fatalf("couldn't form http request: %v", err)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Errorf("error response: %v", err)
			}
			if got, want := resp.StatusCode, test.wantStatus; got != want {
				body, _ := ioutil.ReadAll(resp.Body)
				t.Errorf("status code got %s, want %d (%s)", resp.Status, want, body)
			}
			defer resp.Body.Close()
			respBody, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Errorf("failed to read response body: %v", err)
			}
			if test.wantCP != nil && !bytes.HasPrefix(respBody, test.wantCP) {
				t.Errorf("got body:\n%s\nbut want:\n%s", respBody, test.wantCP)
			}
		})
	}
}
