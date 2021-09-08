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

package witness

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/serverless/client"
	"golang.org/x/mod/sumdb/note"
)

const (
	testOrigin = "test-origin"
)

func TestCheckpointNConsensus(t *testing.T) {
	logS, logV := genKeyPair(t, "log")
	wit1S, wit1V := genKeyPair(t, "w1")
	wit2S, wit2V := genKeyPair(t, "w2")
	wit3S, wit3V := genKeyPair(t, "w3")
	logID := "test-log"
	checkpointPath := func(i int) string { return fmt.Sprintf("logs/%s/checkpoint.%d", logID, i) }

	for _, test := range []struct {
		desc         string
		logID        string
		distributors []client.Fetcher
		witnesses    []note.Verifier
		N            int
		wantErr      bool
		wantCP       []byte
	}{
		{
			desc:  "works N=1",
			logID: logID,
			N:     2,
			distributors: []client.Fetcher{
				fetcher(map[string][]byte{
					checkpointPath(2): newCP(t, 11, logS, wit1S, wit2S, wit3S),
				}),
			},
			witnesses: []note.Verifier{wit1V, wit2V, wit3V},
			wantCP:    newCP(t, 11, logS, wit1S, wit2S, wit3S),
		},
		{
			desc:  "works N=3",
			logID: logID,
			N:     3,
			distributors: []client.Fetcher{
				fetcher(map[string][]byte{
					checkpointPath(3): newCP(t, 11, logS, wit1S, wit2S, wit3S),
				}),
			},
			witnesses: []note.Verifier{wit1V, wit2V, wit3V},
			wantCP:    newCP(t, 11, logS, wit1S, wit2S, wit3S),
		},
		{
			desc:  "largest CP across distributors",
			logID: logID,
			N:     2,
			distributors: []client.Fetcher{
				fetcher(map[string][]byte{
					checkpointPath(2): newCP(t, 11, logS, wit1S, wit2S, wit3S),
				}),
				fetcher(map[string][]byte{
					checkpointPath(2): newCP(t, 15, logS, wit1S, wit2S, wit3S),
				}),
			},
			witnesses: []note.Verifier{wit1V, wit2V, wit3V},
			wantCP:    newCP(t, 15, logS, wit1S, wit2S, wit3S),
		},
		{
			desc:  "largest CP with required sigs",
			logID: logID,
			N:     2,
			distributors: []client.Fetcher{
				fetcher(map[string][]byte{
					checkpointPath(2): newCP(t, 11, logS, wit1S, wit2S, wit3S),
				}),
				fetcher(map[string][]byte{
					checkpointPath(2): newCP(t, 15, logS, wit1S),
				}),
			},
			witnesses: []note.Verifier{wit1V, wit2V, wit3V},
			wantCP:    newCP(t, 11, logS, wit1S, wit2S, wit3S),
		},
		{
			desc:  "err: no files",
			logID: logID,
			N:     2,
			distributors: []client.Fetcher{
				fetcher(map[string][]byte{}),
				fetcher(map[string][]byte{}),
			},
			witnesses: []note.Verifier{wit1V, wit2V, wit3V},
			wantErr:   true,
		},
		{
			desc:  "err: unsatisfiable - not enough sigs",
			logID: logID,
			N:     20,
			distributors: []client.Fetcher{
				fetcher(map[string][]byte{
					checkpointPath(20): newCP(t, 11, logS, wit1S, wit2S, wit3S),
				}),
			},
			witnesses: []note.Verifier{wit1V, wit2V, wit3V},
			wantErr:   true,
		},
		{
			desc:  "err: unsatisfiable - unknown witnesses",
			logID: logID,
			N:     2,
			distributors: []client.Fetcher{
				fetcher(map[string][]byte{
					checkpointPath(2): newCP(t, 11, logS, wit1S, wit2S, wit3S),
				}),
			},
			witnesses: []note.Verifier{wit1V},
			wantErr:   true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			f := CheckpointNConsensus(test.logID, test.distributors, test.witnesses, test.N)
			_, raw, err := f(context.Background(), logV, testOrigin)
			gotErr := err != nil
			if gotErr != test.wantErr {
				t.Errorf("Got err: %v, want err: %v", err, test.wantErr)
			}
			if got, want := string(raw), string(test.wantCP); got != want {
				t.Errorf("got CP:\n%s\nWant:\n%s", got, want)
			}
		})
		glog.Flush()
	}
}

func fetcher(m map[string][]byte) client.Fetcher {
	mf := mapFetcher(m)
	return mf.Fetch
}

type mapFetcher map[string][]byte

func (f mapFetcher) Fetch(_ context.Context, p string) ([]byte, error) {
	v, ok := f[p]
	if !ok {
		return nil, fmt.Errorf("%q: %v", p, os.ErrNotExist)
	}
	return v, nil
}

func newCP(t *testing.T, size int, sigs ...note.Signer) []byte {
	t.Helper()
	cp := log.Checkpoint{
		Origin: testOrigin,
		Size:   uint64(size),
		Hash:   []byte("banana"),
	}
	ret, err := note.Sign(&note.Note{Text: string(cp.Marshal())}, sigs...)
	if err != nil {
		t.Fatalf("Failed to sign note: %v", err)
	}
	return ret
}

func genKeyPair(t *testing.T, name string) (note.Signer, note.Verifier) {
	t.Helper()
	sKey, vKey, err := note.GenerateKey(nil, name)
	if err != nil {
		t.Fatalf("Failed to generate key %q: %v", name, err)
	}
	s, err := note.NewSigner(sKey)
	if err != nil {
		t.Fatalf("Failed to create signer %q: %v", name, err)
	}
	v, err := note.NewVerifier(vKey)
	if err != nil {
		t.Fatalf("Failed to create verifier %q: %v", name, err)
	}
	return s, v
}
