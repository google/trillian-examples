// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package audit

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"golang.org/x/mod/sumdb/tlog"
)

const (
	leafData = `golang.org/x/net v0.0.0-20180627171509-e514e69ffb8b h1:oXs/nlnyk1ue6g+mFGEHIuIaQIT28IgumdSIRMq2aJY=
golang.org/x/net v0.0.0-20180627171509-e514e69ffb8b/go.mod h1:mL1N/T3taQHkDXs73rZJwtUhF3w3ftmwwsq0BUmARs4=

golang.org/x/exp/notary v0.0.0-20190409044807-56b785ea58b2 h1:f//7NxweD0pr6lfEPxg1OgwjMgrBOuLknHTUue6T60s=
golang.org/x/exp/notary v0.0.0-20190409044807-56b785ea58b2/go.mod h1:LX2MmXhzwauji5+hdOp9+fZiT7bHVjDavhHX8IDQ4T0=

golang.org/x/crypto v0.0.0-20181025213731-e84da0312774 h1:a4tQYYYuK9QdeO/+kEvNYyuR21S+7ve5EANok6hABhI=
golang.org/x/crypto v0.0.0-20181025213731-e84da0312774/go.mod h1:6SG95UA2DQfeDnfUPMdvaQW0Q7yPrPDi9nlGo2tz2b4=

golang.org/x/madethisup v0.0.0-1 h1:a4tQYYYuK9QdeO/+kEvNYyuR21S+7ve5EANok6hABhI=
golang.org/x/madethisup v0.0.0-1/go.mod h1:6SG95UA2DQfeDnfUPMdvaQW0Q7yPrPDi9nlGo2tz2b4=
`

	checkpointData = `go.sum database tree
1514086
kn9DgqDhXzoZMM8828SQsbuovr/WRn7QfFd5Qe1rpwA=

— sum.golang.org Az3grunuggF5mKymPJeK/l9Pq71lOg/rAVkQVCzGkWRJcnS3ZFunzveHr9PAH8LFsuhpcCWzGDNrn9FFDyXm/66tBg8=
`

	tileHashData = `d7b9018cbad2a2fa3950dcd60411cd67ef9d8c1074043c0e033953ec510fd68413f83190fb460efeb65670f9298b4249b8b5fd2492a6cd486f1fe14bfa3eb545590ac0de6fc0f9b016875ba518353cc57654df733f2fa1d1f0dad66f84b66d9ea2744fc0a64bb00d9f286c52838284b4b76bcbe895854d4709c55df1c266b681`
)

func TestLeavesAtOffset(t *testing.T) {
	sumdb := &SumDBClient{
		vkey:   "sum.golang.org+033de0ae+Ac4zctda0e5eza+HJyk9SxEdh+s3Ux18htTTAD8OuAn8",
		height: 2,
		fetcher: &FakeFetcher{
			values: map[string]string{"/tile/2/data/000": leafData},
		},
	}
	leaves, err := sumdb.FullLeavesAtOffset(0)
	if err != nil {
		t.Fatalf("failed to get leaves: %v", err)
	}
	if got, want := len(leaves), 4; got != want {
		t.Errorf("got: %d, want: %d", got, want)
	}
	for _, l := range leaves {
		if l[len(l)-1] != '\n' {
			t.Errorf("expected string terminating in newline, got: %x", l)
		}
		expStart := "golang.org/x/"
		if got, want := fmt.Sprintf("%s", l[:len(expStart)]), expStart; got != want {
			t.Errorf("got prefix '%s', wanted '%s'", got, want)
		}
	}
}

func TestLatestCheckpoint(t *testing.T) {
	sumdb := &SumDBClient{
		vkey:   "sum.golang.org+033de0ae+Ac4zctda0e5eza+HJyk9SxEdh+s3Ux18htTTAD8OuAn8",
		height: 2,
		fetcher: &FakeFetcher{
			values: map[string]string{"/latest": checkpointData},
		},
	}
	checkpoint, err := sumdb.LatestCheckpoint()
	if err != nil {
		t.Fatalf("failed to get checkpoint: %v", err)
	}
	if !bytes.Equal(checkpoint.Raw, []byte(checkpointData)) {
		t.Errorf("expected raw checkpoint %q but got %q", checkpointData, checkpoint.Raw)
	}
	expectedHash, err := tlog.ParseHash("kn9DgqDhXzoZMM8828SQsbuovr/WRn7QfFd5Qe1rpwA=")
	if err != nil {
		t.Fatalf("static hash failed to parse")
	}
	if got, want := checkpoint.Hash, expectedHash; got != want {
		t.Errorf("unexpected hash. got, want = %s, %s", got, want)
	}
	if got, want := checkpoint.N, int64(1514086); got != want {
		t.Errorf("unexpected tree size. got, want = %d, %d", got, want)
	}
}

func TestTileHashes(t *testing.T) {
	hashData, err := hex.DecodeString(tileHashData)
	if err != nil {
		t.Fatalf("failed to decode hash data: %v", err)
	}
	sumdb := &SumDBClient{
		vkey:   "sum.golang.org+033de0ae+Ac4zctda0e5eza+HJyk9SxEdh+s3Ux18htTTAD8OuAn8",
		height: 2,
		fetcher: &FakeFetcher{
			values: map[string]string{"/tile/2/0/000": string(hashData)},
		},
	}
	hashes, err := sumdb.TileHashes(0, 0, 0)
	if err != nil {
		t.Fatalf("failed to get hashes: %v", err)
	}
	if got, want := len(hashes), 4; got != want {
		t.Errorf("got, want = %d, %d", got, want)
	}
}

type FakeFetcher struct {
	values map[string]string
}

func (f *FakeFetcher) GetData(path string) ([]byte, error) {
	res, ok := f.values[path]
	if !ok {
		return nil, fmt.Errorf("could not find '%s'", path)
	}
	return []byte(res), nil
}
