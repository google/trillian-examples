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

// rekor_feeder is an implementation of a witness feeder for the Sigstore log: Rekór.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/internal/feeder"
	"github.com/google/trillian-examples/serverless/config"
	"golang.org/x/mod/sumdb/note"

	i_note "github.com/google/trillian-examples/internal/note"
	wit_http "github.com/google/trillian-examples/witness/golang/client/http"
	yaml "gopkg.in/yaml.v2"
)

var (
	configFile = flag.String("config_file", "", "Path to feeder config file.")
	timeout    = flag.Duration("timeout", 10*time.Second, "Maximum time to wait for witnesses to respond.")
	interval   = flag.Duration("interval", time.Duration(0), "Interval between attempts to feed checkpoints. Default of 0 causes the tool to be a one-shot.")
)

// Config encapsulates the feeder config.
type Config struct {
	// Logs defines the source logs to feed from.
	Logs []config.Log `yaml:"Logs"`

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

	wg := &sync.WaitGroup{}
	ctx := context.Background()
	for _, l := range cfg.Logs {
		wg.Add(1)
		go func(l config.Log, w wit_http.Witness) {
			defer wg.Done()
			if err := feedLog(ctx, l, witness, *timeout, *interval); err != nil {
				glog.Errorf("feedLog: %v", err)
			}
		}(l, witness)
	}
	wg.Wait()
}

// logInfo is a partial representation of the JSON object returned by Rekór's
// api/v1/log request.
type logInfo struct {
	// SignedTreeHead contains a Rekór checkpoint.
	SignedTreeHead string `json:"signedTreeHead"`
}

// proof is a partial representation of the JSON struct returned by the Rekór
// api/v1/log/proof request.
type proof struct {
	Hashes [][]byte `json:"hashes"`
}

func feedLog(ctx context.Context, l config.Log, w wit_http.Witness, timeout time.Duration, interval time.Duration) error {
	lURL, err := url.Parse(l.URL)
	if err != nil {
		return fmt.Errorf("invalid LogURL %q: %v", l.URL, err)
	}

	fetchCP := func(ctx context.Context) ([]byte, error) {
		li := logInfo{}
		if err := getJSON(ctx, lURL, "api/v1/log", &li); err != nil {
			return nil, fmt.Errorf("failed to fetch log info: %v", err)
		}
		return []byte(li.SignedTreeHead), nil
	}
	fetchProof := func(ctx context.Context, from, to log.Checkpoint) ([][]byte, error) {
		if from.Size == 0 {
			return [][]byte{}, nil
		}
		cp := proof{}
		if err := getJSON(ctx, lURL, fmt.Sprintf("api/v1/log/proof?firstSize=%d&lastSize=%d", from.Size, to.Size), &cp); err != nil {
			return nil, fmt.Errorf("failed to fetch log info: %v", err)
		}
		return cp.Hashes, nil
	}

	opts := feeder.FeedOpts{
		LogID:           l.ID,
		LogOrigin:       l.Origin,
		FetchCheckpoint: fetchCP,
		FetchProof:      fetchProof,
		LogSigVerifier:  mustCreateVerifier(l.PublicKeyType, l.PublicKey),
		Witness:         w,
	}
	if interval > 0 {
		return feeder.Run(ctx, interval, opts)
	}
	_, err = feeder.FeedOnce(ctx, opts)
	return err
}

func getJSON(ctx context.Context, base *url.URL, path string, s interface{}) error {
	u, err := base.Parse(path)
	if err != nil {
		return fmt.Errorf("failed to parse URL: %v", err)
	}
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}
	req = req.WithContext(ctx)

	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request to %q: %v", u.String(), err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode == 404 {
		return os.ErrNotExist
	}
	if rsp.StatusCode != 200 {
		return fmt.Errorf("unexpected status fetching %q: %s", u.String(), rsp.Status)
	}

	raw, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return fmt.Errorf("failed to read body from %q: %v", u.String(), err)
	}
	if err := json.Unmarshal(raw, s); err != nil {
		glog.Infof("Got body:\n%s", string(raw))
		return fmt.Errorf("failed to unmarshal JSON: %v", err)
	}
	return nil
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
	for i := range cfg.Logs {
		if cfg.Logs[i].ID == "" {
			cfg.Logs[i].ID = log.ID(cfg.Logs[i].Origin, []byte(cfg.Logs[i].PublicKey))
		}
	}
	return &cfg, nil
}
