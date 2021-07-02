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

package client

import (
	"context"
	"encoding/base64"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian/merkle/rfc6962/hasher"
)

var (
	// Built using serverless/testdata/build_log.sh
	testCheckpoints = []log.Checkpoint{
		log.Checkpoint{
			Size: 1,
			Hash: b64("0Nc2CrefWKseHj/mStd+LqC8B+NrX0btIiPt2SmN+ek="),
		},
		log.Checkpoint{
			Size: 2,
			Hash: b64("T1X2GdkhUjV3iyufF9b0kVsWFxIU0VI4EpNml2Teci4="),
		},
		log.Checkpoint{
			Size: 3,
			Hash: b64("Wqx3HImawpLnS/Gv4ubjAvi1WIOy0b8Ze0amvqbavKk="),
		},
		log.Checkpoint{
			Size: 4,
			Hash: b64("zY1lN35vrXYAPixXSd59LsU29xUJtuW4o2dNNg5Y2Co="),
		},
		log.Checkpoint{
			Size: 5,
			Hash: b64("gy5gl3aksFyiCO95a/1vLXz88A3dRq+0l9Sxte8ZqZQ="),
		},
		log.Checkpoint{
			Size: 6,
			Hash: b64("a6sWvsc2eEzmj72vah7mZ5dwFltivehh2b11qwlp5Jg="),
		},
		log.Checkpoint{
			Size: 7,
			Hash: b64("IrSXADBqJ7EQoUODSDKROySgNveeL6CFhik2w/+fS7U="),
		},
	}
)

func b64(r string) []byte {
	ret, err := base64.StdEncoding.DecodeString(r)
	if err != nil {
		panic(err)
	}
	return ret
}

func TestCheckConsistency(t *testing.T) {
	ctx := context.Background()

	h := hasher.DefaultHasher

	f := func(_ context.Context, p string) ([]byte, error) {
		path := filepath.Join("../testdata/log", p)
		return ioutil.ReadFile(path)
	}
	for _, test := range []struct {
		desc    string
		cp      []log.Checkpoint
		wantErr bool
	}{
		{
			desc: "2 CP",
			cp: []log.Checkpoint{
				testCheckpoints[2],
				testCheckpoints[5],
			},
		}, {
			desc: "5 CP",
			cp: []log.Checkpoint{
				testCheckpoints[0],
				testCheckpoints[2],
				testCheckpoints[3],
				testCheckpoints[5],
				testCheckpoints[6],
			},
		}, {
			desc: "Identical CP",
			cp: []log.Checkpoint{
				testCheckpoints[0],
				testCheckpoints[0],
				testCheckpoints[0],
				testCheckpoints[0],
			},
		}, {
			desc: "Identical CP pairs",
			cp: []log.Checkpoint{
				testCheckpoints[0],
				testCheckpoints[0],
				testCheckpoints[5],
				testCheckpoints[5],
			},
		}, {
			desc: "Out of order",
			cp: []log.Checkpoint{
				testCheckpoints[5],
				testCheckpoints[2],
				testCheckpoints[0],
				testCheckpoints[3],
			},
		}, {
			desc: "Inconsistent",
			cp: []log.Checkpoint{
				testCheckpoints[5],
				testCheckpoints[2],
				log.Checkpoint{
					Size: 4,
					Hash: []byte("This is a banana"),
				},
				testCheckpoints[3],
			},
			wantErr: true,
		}, {
			desc: "Inconsistent - clashing CPs",
			cp: []log.Checkpoint{
				log.Checkpoint{
					Size: 2,
					Hash: []byte("This is a banana"),
				},
				log.Checkpoint{
					Size: 2,
					Hash: []byte("This is NOT a banana"),
				},
			},
			wantErr: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			err := CheckConsistency(ctx, h, f, test.cp)
			if gotErr := err != nil; gotErr != test.wantErr {
				t.Fatalf("wantErr: %t, got %v", test.wantErr, err)
			}
		})
	}
}
