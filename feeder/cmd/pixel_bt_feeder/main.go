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

// pixel_bt_feeder is an implementation of a witness feeder for the Pixel BT log.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/internal/feeder"
	"github.com/google/trillian-examples/serverless/config"
	"golang.org/x/mod/sumdb/note"
	"golang.org/x/mod/sumdb/tlog"

	i_note "github.com/google/trillian-examples/internal/note"
	wit_http "github.com/google/trillian-examples/witness/golang/client/http"
	yaml "gopkg.in/yaml.v2"
)

const (
	// tileHeight is the tlog tile height.
	// From: https://developers.devsite.corp.google.com/android/binary_transparency/tile
	tileHeight = 1
)

var (
	configFile = flag.String("config_file", "", "Path to feeder config file.")
	timeout    = flag.Duration("timeout", 10*time.Second, "Maximum time to wait for witnesses to respond.")
	interval   = flag.Duration("interval", time.Duration(0), "Interval between attempts to feed checkpoints. Default of 0 causes the tool to be a one-shot.")
)

// Config encapsulates the feeder config.
type Config struct {
	// Log defines the source log to feed from.
	Log config.Log `yaml:"Log"`

	// Witness is the configured witness.
	Witness config.Witness `yaml:"Witness"`
}

func main() {
	flag.Parse()

	cfg, err := readConfig(*configFile)
	if err != nil {
		glog.Exitf("Failed to read config: %v", err)
	}

	u, err := url.Parse(cfg.Witness.URL)
	if err != nil {
		glog.Exitf("Failed to parse witness URL %q: %v", cfg.Witness.URL, err)
	}
	witness := wit_http.Witness{
		URL:      u,
		Verifier: mustCreateVerifier(i_note.Note, cfg.Witness.PublicKey),
	}

	ctx := context.Background()
	if err := feedLog(ctx, cfg.Log, witness, *timeout, *interval); err != nil {
		glog.Errorf("feedLog: %v", err)
	}
}

// convertToNote converts a the PixelBT checkpoint to a valid Note.
//
// Hopefully we won't need this for too long, and PixelBT will update their checkpoint format to make it
// fully compatible.
func convertToNote(pixelCP, keyName string, keyHash uint32) ([]byte, error) {
	split := strings.LastIndex(pixelCP, "\n\n")
	if split < 0 {
		return nil, fmt.Errorf("invalid Pixel CP %q", pixelCP)
	}
	cpBody, sigB64 := pixelCP[:split+1], pixelCP[split+2:]

	if !strings.HasSuffix(cpBody, "\n") {
		return nil, errors.New("checkpoint body does not end with a newline")
	}
	validName := keyName != "" && utf8.ValidString(keyName) && strings.IndexFunc(keyName, unicode.IsSpace) < 0 && !strings.Contains(keyName, "+")
	if !validName {
		return nil, fmt.Errorf("invalid keyname %q", keyName)
	}
	if len(sigB64) == 0 {
		return nil, errors.New("invalid checkpoint - empty signature")
	}
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 in signature %q: %v", sigB64, err)
	}

	var buf bytes.Buffer
	buf.WriteString(cpBody)
	buf.WriteString("\n")

	var hbuf [4]byte
	binary.BigEndian.PutUint32(hbuf[:], keyHash)
	b64 := base64.StdEncoding.EncodeToString(append(hbuf[:], []byte(sig)...))
	buf.WriteString("— ")
	buf.WriteString(keyName)
	buf.WriteString(" ")
	buf.WriteString(b64)
	buf.WriteString("\n")
	return buf.Bytes(), nil
}

func feedLog(ctx context.Context, l config.Log, w wit_http.Witness, timeout time.Duration, interval time.Duration) error {
	lURL, err := url.Parse(l.URL)
	if err != nil {
		return fmt.Errorf("invalid LogURL %q: %v", l.URL, err)
	}
	logSigV := mustCreateVerifier(l.PublicKeyType, l.PublicKey)

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
		r := make([][]byte, 0, len(h))
		for _, h := range proof {
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

func mustCreateVerifier(t, pub string) note.Verifier {
	v, err := i_note.NewVerifier(t, pub)
	if err != nil {
		glog.Exitf("Failed to create signature verifier from %q: %v", pub, err)
	}
	return v
}
func readConfig(f string) (*Config, error) {
	c, err := ioutil.ReadFile(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}
	cfg := Config{}
	if err := yaml.Unmarshal(c, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %v", err)
	}
	if cfg.Log.ID == "" {
		cfg.Log.ID = log.ID(cfg.Log.Origin, []byte(cfg.Log.PublicKey))
	}
	return &cfg, nil
}
