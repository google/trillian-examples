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

package feeder

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/serverless/client"
	"github.com/google/trillian-examples/serverless/testdata"
	"github.com/google/trillian/merkle/rfc6962"
	"golang.org/x/mod/sumdb/note"
)

const (
	testWitnessSecretKey = "PRIVATE+KEY+test-witness+d28ecb0d+AbextYkHs2Be69xhwhvaUvjxijEqtQQ9NWNC05YWx7Jr"
	testWitnessPublicKey = "test-witness+d28ecb0d+AV2DM8GnnoXPS77FKY/KwsZdfien9eSmb35f6qUv2TrH"
)

func TestFeedOnce(t *testing.T) {
	ctx := context.Background()
	logSigV := testdata.LogSigVerifier(t)
	witSig := mustCreateSigner(t, testWitnessSecretKey)
	witSigV := mustCreateVerifier(t, testWitnessPublicKey)
	for _, test := range []struct {
		desc     string
		submitCP []byte
		witness  Witness
		wantErr  bool
	}{
		{
			desc:     "works",
			submitCP: testdata.Checkpoint(t, 2),
			witness: &fakeWitness{
				logSigV:  logSigV,
				witSig:   witSig,
				witSigV:  witSigV,
				latestCP: mustCosignCP(t, testdata.Checkpoint(t, 1), logSigV, witSig),
			},
		}, {
			desc:     "works - TOFU feed",
			submitCP: testdata.Checkpoint(t, 2),
			witness: &fakeWitness{
				logSigV: logSigV,
				witSig:  witSig,
				witSigV: witSigV,
			},
		}, {
			desc:     "works - submitCP == latest",
			submitCP: testdata.Checkpoint(t, 1),
			witness: &fakeWitness{
				logSigV:  logSigV,
				witSig:   witSig,
				witSigV:  witSigV,
				latestCP: mustCosignCP(t, testdata.Checkpoint(t, 1), logSigV, witSig),
			},
		}, {
			desc:     "no new sigs added",
			submitCP: mustCosignCP(t, testdata.Checkpoint(t, 1), logSigV, witSig),
			witness: &fakeWitness{
				logSigV:  logSigV,
				witSig:   witSig,
				witSigV:  witSigV,
				latestCP: mustCosignCP(t, testdata.Checkpoint(t, 1), logSigV, witSig),
			},
			wantErr: true,
		}, {
			desc:    "submitting stale CP",
			wantErr: true,
			// This checkpoint is older than the witness' latestCP below:
			submitCP: testdata.Checkpoint(t, 1),
			witness: &fakeWitness{
				logSigV:  logSigV,
				witSig:   witSig,
				witSigV:  witSigV,
				latestCP: mustCosignCP(t, testdata.Checkpoint(t, 2), logSigV, witSig),
			},
		},
	} {
		sCP := mustOpenCheckpoint(t, test.submitCP, testdata.TestLogOrigin, testdata.LogSigVerifier(t))
		f := testdata.HistoryFetcher(sCP.Size)
		fetchCheckpoint := func(_ context.Context) ([]byte, error) {
			return test.submitCP, nil
		}
		fetchProof := func(ctx context.Context, from, to log.Checkpoint) ([][]byte, error) {
			if from.Size == 0 {
				return [][]byte{}, nil
			}
			pb, err := client.NewProofBuilder(ctx, to, rfc6962.DefaultHasher.HashChildren, f.Fetcher())
			if err != nil {
				return nil, fmt.Errorf("failed to create proof builder: %v", err)
			}

			conP, err := pb.ConsistencyProof(ctx, from.Size, to.Size)
			if err != nil {
				return nil, fmt.Errorf("failed to create proof for (%d -> %d): %v", from.Size, to.Size, err)
			}
			return conP, nil
		}

		opts := FeedOpts{
			FetchCheckpoint: fetchCheckpoint,
			FetchProof:      fetchProof,
			LogOrigin:       testdata.TestLogOrigin,
			LogSigVerifier:  testdata.LogSigVerifier(t),
			Witness:         test.witness,
		}
		t.Run(test.desc, func(t *testing.T) {
			_, err := FeedOnce(ctx, opts)
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
	witSigV      note.Verifier
	latestCP     []byte
	rejectUpdate bool
}

func (fw *fakeWitness) SigVerifier() note.Verifier {
	return fw.witSigV
}

func (fw *fakeWitness) GetLatestCheckpoint(_ context.Context, logID string) ([]byte, error) {
	if fw.latestCP == nil {
		return nil, os.ErrNotExist
	}
	return fw.latestCP, nil
}

func (fw *fakeWitness) Update(_ context.Context, logID string, newCP []byte, proof [][]byte) ([]byte, error) {
	if fw.rejectUpdate {
		return nil, errors.New("computer says 'no'")
	}

	csCP, err := cosignCP(newCP, fw.logSigV, fw.witSig)
	if err != nil {
		return nil, err
	}
	fw.latestCP = csCP

	return fw.latestCP, nil
}

func mustOpenCheckpoint(t *testing.T, cp []byte, origin string, v note.Verifier) log.Checkpoint {
	t.Helper()
	c, _, _, err := log.ParseCheckpoint(cp, origin, v)
	if err != nil {
		t.Fatalf("Failed to open checkpoint: %v", err)
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

func mustCosignCP(t *testing.T, cp []byte, v note.Verifier, s note.Signer) []byte {
	t.Helper()
	r, err := cosignCP(cp, v, s)
	if err != nil {
		t.Fatalf("Failed to cosign CP: %v", err)
	}
	return r
}

func mustCreateSigner(t *testing.T, secK string) note.Signer {
	t.Helper()
	s, err := note.NewSigner(secK)
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}
	return s
}
func mustCreateVerifier(t *testing.T, pubK string) note.Verifier {
	t.Helper()
	v, err := note.NewVerifier(pubK)
	if err != nil {
		t.Fatalf("failed to create verifier: %v", err)
	}
	return v
}
