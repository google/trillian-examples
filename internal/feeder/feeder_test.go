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
	"github.com/transparency-dev/merkle/rfc6962"
	"golang.org/x/mod/sumdb/note"
)

func TestFeedOnce(t *testing.T) {
	ctx := context.Background()
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
				latestCP: testdata.Checkpoint(t, 1),
			},
		}, {
			desc:     "works after a few failures",
			submitCP: testdata.Checkpoint(t, 2),
			witness: &slowWitness{
				fakeWitness: &fakeWitness{
					latestCP: testdata.Checkpoint(t, 1),
				},
				times: 3,
			},
		}, {
			desc:     "works - TOFU feed",
			submitCP: testdata.Checkpoint(t, 2),
			witness: &fakeWitness{
			},
		}, {
			desc:     "works - submitCP == latest",
			submitCP: testdata.Checkpoint(t, 1),
			witness: &fakeWitness{
				latestCP: testdata.Checkpoint(t, 1),
			},
		}, {
			desc:    "submitting stale CP",
			wantErr: true,
			// This checkpoint is older than the witness' latestCP below:
			submitCP: testdata.Checkpoint(t, 1),
			witness: &fakeWitness{
				latestCP: testdata.Checkpoint(t, 2),
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

type slowWitness struct {
	*fakeWitness
	times int
}

func (sw *slowWitness) GetLatestCheckpoint(ctx context.Context, logID string) ([]byte, error) {
	if sw.times > 0 {
		sw.times = sw.times - 1
		return nil, fmt.Errorf("will fail for %d more calls", sw.times)
	}
	return sw.fakeWitness.GetLatestCheckpoint(ctx, logID)
}

type fakeWitness struct {
	latestCP     []byte
	rejectUpdate bool
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

	fw.latestCP = newCP

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