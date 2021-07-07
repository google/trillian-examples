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
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/serverless/testdata"
	"golang.org/x/mod/sumdb/note"
)

const (
	testWitnessSecretKey = "PRIVATE+KEY+test-witness+d28ecb0d+AbextYkHs2Be69xhwhvaUvjxijEqtQQ9NWNC05YWx7Jr"
	// Unused, but seems sensible to keep it documented here:
	// testWitnessPublicKey = "test-witness+d28ecb0d+AV2DM8GnnoXPS77FKY/KwsZdfien9eSmb35f6qUv2TrH"
)

func TestWitness(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		desc        string
		submitCP    []byte
		numRequired int
		witnesses   []Witness
		wantErr     bool
	}{
		{
			desc:        "works",
			submitCP:    testdata.Checkpoint(t, 2),
			numRequired: 1,
			witnesses: []Witness{
				&fakeWitness{
					logSigV:  testdata.LogSigVerifier(t),
					witSig:   mustCreateSigner(t, testWitnessSecretKey),
					latestCP: testdata.Checkpoint(t, 1),
				},
			},
		}, {
			desc:        "works - submitCP == latest",
			submitCP:    testdata.Checkpoint(t, 1),
			numRequired: 1,
			witnesses: []Witness{
				&fakeWitness{
					logSigV:  testdata.LogSigVerifier(t),
					witSig:   mustCreateSigner(t, testWitnessSecretKey),
					latestCP: testdata.Checkpoint(t, 1),
				},
			},
		}, {
			desc:    "submitting stale CP",
			wantErr: true,
			// This checkpoint is older than the witness' latestCP below:
			submitCP:    testdata.Checkpoint(t, 1),
			numRequired: 1,
			witnesses: []Witness{
				&fakeWitness{
					logSigV:  testdata.LogSigVerifier(t),
					witSig:   mustCreateSigner(t, testWitnessSecretKey),
					latestCP: testdata.Checkpoint(t, 2),
				},
			},
		}, {
			desc:    "invalid opts - numRequired > num witnesses",
			wantErr: true,
			// Whoops:
			numRequired: 100,
			submitCP:    testdata.Checkpoint(t, 1),
			witnesses:   []Witness{&fakeWitness{}},
		},
	} {
		sCP := mustOpenCheckpoint(t, test.submitCP, testdata.LogSigVerifier(t))
		f := testdata.HistoryFetcher(sCP.Size)
		opts := FeedOpts{
			LogFetcher:     f.Fetcher(),
			LogSigVerifier: testdata.LogSigVerifier(t),
			NumRequired:    test.numRequired,
			Witnesses:      test.witnesses,
		}
		t.Run(test.desc, func(t *testing.T) {
			_, err := Feed(ctx, test.submitCP, opts)
			gotErr := err != nil
			if test.wantErr != gotErr {
				t.Fatalf("Got err %v, want err %t", err, test.wantErr)
			}
		})
	}
}

type fakeWitness struct {
	logSigV      note.Verifier
	witSig       note.Signer
	latestCP     []byte
	rejectUpdate bool
}

func (fw *fakeWitness) SigVerifier() note.Verifier {
	return fw.logSigV
}

func (fw *fakeWitness) GetLatestCheckpoint(_ context.Context, logID string) ([]byte, error) {
	if fw.latestCP == nil {
		return nil, os.ErrNotExist
	}
	return fw.latestCP, nil
}

func (fw *fakeWitness) Update(_ context.Context, logID string, newCP []byte, proof [][]byte) error {
	if fw.rejectUpdate {
		return errors.New("computer says 'no'")
	}

	csCP, err := cosignCP(newCP, fw.logSigV, fw.witSig)
	if err != nil {
		return err
	}
	fw.latestCP = csCP

	return nil
}

func mustOpenCheckpoint(t *testing.T, cp []byte, v note.Verifier) log.Checkpoint {
	t.Helper()
	n, err := note.Open(cp, note.VerifierList(v))
	if err != nil {
		t.Fatalf("Failed to open checkpoint: %v", err)
	}
	c := &log.Checkpoint{}
	_, err = c.Unmarshal([]byte(n.Text))
	if err != nil {
		t.Fatalf("Failed to unmarshall checkpoint: %v", err)
	}
	return *c
}

func cosignCP(cp []byte, v note.Verifier, s note.Signer) ([]byte, error) {
	n, err := note.Open(cp, note.VerifierList(v))
	if err != nil {
		return nil, fmt.Errorf("failed to open note: %v", err)
	}

	r, err := note.Sign(n, s)
	if err != nil {
		return nil, fmt.Errorf("failed to cosign note: %v", err)
	}
	return r, nil
}
func mustCreateSigner(t *testing.T, secK string) note.Signer {
	t.Helper()
	s, err := note.NewSigner(secK)
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}
	return s
}
