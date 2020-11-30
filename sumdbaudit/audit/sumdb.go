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
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"golang.org/x/mod/sumdb/note"
	"golang.org/x/mod/sumdb/tlog"
)

// HashLenBytes is the number of bytes in the SumDB hashes.
const HashLenBytes = 32

// pathBase is the max number of entries in any SumDB directory.
// Beyond this, sub directories are created.
// https://github.com/golang/mod/blob/346a37af5599be02f125bd8bc0a5e1d33db21ddc/sumdb/tlog/tile.go#L168
const pathBase = 1000

// Fetcher gets data paths. This allows impl to be swapped for tests.
type Fetcher interface {
	// GetData gets the data at the given path, or returns an error.
	GetData(path string) ([]byte, error)
}

// Checkpoint is a parsed Checkpoint and the raw bytes that it came from.
type Checkpoint struct {
	*tlog.Tree

	Raw []byte
}

// SumDBClient provides access to information from the Sum DB.
type SumDBClient struct {
	height  int
	vkey    string
	fetcher Fetcher
}

// NewSumDB creates a new client that fetches tiles of the given height.
func NewSumDB(height int, vkey string) *SumDBClient {
	name := vkey
	if i := strings.Index(name, "+"); i >= 0 {
		name = name[:i]
	}
	target := "https://" + name

	return &SumDBClient{
		height:  height,
		vkey:    vkey,
		fetcher: &HTTPFetcher{baseURL: target},
	}
}

// LatestCheckpoint gets the freshest Checkpoint.
func (c *SumDBClient) LatestCheckpoint() (*Checkpoint, error) {
	checkpoint, err := c.fetcher.GetData("/latest")
	if err != nil {
		return nil, fmt.Errorf("failed to get /latest Checkpoint; %w", err)
	}

	return c.ParseCheckpointNote(checkpoint)
}

// ParseCheckpointNote parses a previously acquired raw checkpoint note data.
func (c *SumDBClient) ParseCheckpointNote(checkpoint []byte) (*Checkpoint, error) {
	verifier, err := note.NewVerifier(c.vkey)
	if err != nil {
		return nil, fmt.Errorf("failed to create verifier: %w", err)
	}
	verifiers := note.VerifierList(verifier)

	note, err := note.Open(checkpoint, verifiers)
	if err != nil {
		return nil, fmt.Errorf("failed to open note: %w", err)
	}
	tree, err := tlog.ParseTree([]byte(note.Text))
	if err != nil {
		return nil, fmt.Errorf("failed to parse tree: %w", err)
	}

	return &Checkpoint{Tree: &tree, Raw: checkpoint}, nil
}

// FullLeavesAtOffset gets the Nth chunk of 2**height leaves.
func (c *SumDBClient) FullLeavesAtOffset(offset int) ([][]byte, error) {
	data, err := c.fetcher.GetData(fmt.Sprintf("/tile/%d/data/%s", c.height, c.tilePath(offset)))
	if err != nil {
		return nil, err
	}
	return dataToLeaves(data), nil
}

// PartialLeavesAtOffset gets the final tile of incomplete leaves.
func (c *SumDBClient) PartialLeavesAtOffset(offset, count int) ([][]byte, error) {
	data, err := c.fetcher.GetData(fmt.Sprintf("/tile/%d/data/%s.p/%d", c.height, c.tilePath(offset), count))
	if err != nil {
		return nil, err
	}
	return dataToLeaves(data), nil
}

// tilePath constructs the component of the path which refers to a tile at a
// given offset. This was copied from the core implementation:
// https://github.com/golang/mod/blob/346a37af5599be02f125bd8bc0a5e1d33db21ddc/sumdb/tlog/tile.go#L171
func (c *SumDBClient) tilePath(offset int) string {
	nStr := fmt.Sprintf("%03d", offset%pathBase)
	for offset >= pathBase {
		offset /= pathBase
		nStr = fmt.Sprintf("x%03d/%s", offset%pathBase, nStr)
	}
	return nStr
}

func dataToLeaves(data []byte) [][]byte {
	result := make([][]byte, 0)
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start && data[i-1] == '\n' {
				result = append(result, data[start:i])
				start = i + 1
			}
		}
	}
	result = append(result, data[start:])
	return result
}

// TileHashes gets the hashes at the given level and offset.
func (c *SumDBClient) TileHashes(level, offset int) ([]tlog.Hash, error) {
	data, err := c.fetcher.GetData(fmt.Sprintf("/tile/%d/%d/%s", c.height, level, c.tilePath(offset)))
	if err != nil {
		return nil, err
	}
	if got, want := len(data), HashLenBytes*1<<c.height; got != want {
		return nil, fmt.Errorf("got %d bytes, expected %d", got, want)
	}
	hashes := make([]tlog.Hash, 1<<c.height)
	for i := 0; i < cap(hashes); i++ {
		var h tlog.Hash
		copy(h[:], data[HashLenBytes*i:HashLenBytes*(i+1)])
		hashes[i] = h
	}
	return hashes, nil
}

// HTTPFetcher gets the data over HTTP(S).
type HTTPFetcher struct {
	baseURL string
}

// GetData gets the data.
func (f *HTTPFetcher) GetData(path string) ([]byte, error) {
	target := f.baseURL + path
	resp, err := http.Get(target)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GET %v: %v", target, resp.Status)
	}
	data, err := ioutil.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return data, nil
}
