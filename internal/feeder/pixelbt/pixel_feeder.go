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

// Package pixelbt is an implementation of a witness feeder for the Pixel BT log.
package pixelbt

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/internal/feeder"
	"github.com/google/trillian-examples/serverless/config"
	"golang.org/x/mod/sumdb/tlog"

	i_note "github.com/google/trillian-examples/internal/note"
	wit_http "github.com/google/trillian-examples/witness/golang/client/http"
)

const (
	// tileHeight is the tlog tile height.
	// From: https://developers.devsite.corp.google.com/android/binary_transparency/tile
	tileHeight = 1
)

// FeedLog retrieves checkpoints and proofs from the source Pixel BT log, and sends them to the witness.
// This method blocks until the context is done.
func FeedLog(ctx context.Context, l config.Log, w wit_http.Witness, timeout time.Duration, interval time.Duration) error {
	lURL, err := url.Parse(l.URL)
	if err != nil {
		return fmt.Errorf("invalid LogURL %q: %v", l.URL, err)
	}
	logSigV, err := i_note.NewVerifier(l.PublicKeyType, l.PublicKey)
	if err != nil {
		return err
	}

	fetchCP := func(ctx context.Context) ([]byte, error) {
		cpTxt, err := fetch(ctx, lURL, "checkpoint.txt")
		if err != nil {
			return nil, fmt.Errorf("failed to fetch checkpoint.txt: %v", err)
		}
		n, err := convertToNote(string(cpTxt), logSigV.Name(), logSigV.KeyHash())
		glog.V(1).Infof("note : %q", n)
		return n, err
	}
	fetchProof := func(ctx context.Context, from, to log.Checkpoint) ([][]byte, error) {
		if from.Size == 0 {
			return [][]byte{}, nil
		}
		var h [32]byte
		copy(h[:], to.Hash)
		tree := tlog.Tree{
			N:    int64(to.Size),
			Hash: h,
		}
		tr := tileReader{fetch: func(p string) ([]byte, error) {
			return fetch(ctx, lURL, p)
		}}

		proof, err := tlog.ProveTree(int64(to.Size), int64(from.Size), tlog.TileHashReader(tree, tr))
		if err != nil {
			return nil, fmt.Errorf("ProveTree: %v", err)
		}
		r := make([][]byte, 0, len(proof))
		for _, h := range proof {
			h := h
			r = append(r, h[:])
		}
		glog.V(1).Infof("Fetched proof from %d -> %d: %x", from.Size, to.Size, r)
		return r, nil
	}

	opts := feeder.FeedOpts{
		LogID:           l.ID,
		LogOrigin:       l.Origin,
		FetchCheckpoint: fetchCP,
		FetchProof:      fetchProof,
		LogSigVerifier:  logSigV,
		Witness:         w,
	}
	if interval > 0 {
		return feeder.Run(ctx, interval, opts)
	}
	_, err = feeder.FeedOnce(ctx, opts)
	return err
}

type tileReader struct {
	fetch func(p string) ([]byte, error)
}

func (tr tileReader) Height() int { return tileHeight }

func (tr tileReader) SaveTiles([]tlog.Tile, [][]byte) {}

func (tr tileReader) ReadTiles(tiles []tlog.Tile) ([][]byte, error) {
	r := make([][]byte, 0, len(tiles))
	for _, t := range tiles {
		path := fmt.Sprintf("tile/%d/%d/%03d", t.H, t.L, t.N)
		if t.W < 1<<t.H {
			path += fmt.Sprintf(".p/%d", t.W)
		}
		tile, err := tr.fetch(path)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch %q: %v", path, err)
		}
		r = append(r, tile)
	}
	return r, nil
}

func fetch(ctx context.Context, base *url.URL, path string) ([]byte, error) {
	u, err := base.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %v", err)
	}
	glog.Infof("GET %s", u.String())
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	req = req.WithContext(ctx)

	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request to %q: %v", u.String(), err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode == 404 {
		return nil, os.ErrNotExist
	}
	if rsp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status fetching %q: %s", u.String(), rsp.Status)
	}

	return ioutil.ReadAll(rsp.Body)
}
