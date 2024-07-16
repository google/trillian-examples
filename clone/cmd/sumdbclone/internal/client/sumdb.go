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

// Package client contains a basic client for the SumDB log.
package client

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/golang/glog"
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
func NewSumDB(height int, vkey, url string, c *http.Client) *SumDBClient {
	return &SumDBClient{
		height: height,
		vkey:   vkey,
		fetcher: &HTTPFetcher{
			c:       c,
			baseURL: url,
		},
	}
}

// LatestCheckpoint gets the freshest Checkpoint.
func (c *SumDBClient) LatestCheckpoint() (*Checkpoint, error) {
	checkpoint, err := c.fetcher.GetData("/latest")
	if err != nil {
		return nil, fmt.Errorf("failed to get /latest Checkpoint; %w", err)
	}

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

// dataToLeaves splits the raw bytes received when requesting a data
// tile into the constituent leaf data chunks. The splitting is performed
// based on two consecutive newlines, and each returned leaf terminates
// with a single newline.
func dataToLeaves(data []byte) [][]byte {
	leaves := bytes.Split(data, []byte{'\n', '\n'})
	for i, l := range leaves {
		leaves[i] = append(bytes.TrimSpace(l), '\n')
	}
	return leaves
}

// HTTPFetcher gets the data over HTTP(S).
type HTTPFetcher struct {
	c       *http.Client
	baseURL string
}

// GetData gets the data.
func (f *HTTPFetcher) GetData(path string) ([]byte, error) {
	target := f.baseURL + path
	resp, err := f.c.Get(target)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			glog.Errorf("resp.Body.Close(): %v", err)
		}
	}()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GET %v: %v", target, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}
