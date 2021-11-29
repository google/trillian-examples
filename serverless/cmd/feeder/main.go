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

// feeder is an implementation of a witness feeder for serverless logs.
// It can be configured to feed from one or more serverless logs to a single witness.
//
// TODO(al): Consider whether to add support for multiple witnesses.
package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/internal/feeder"
	"github.com/google/trillian-examples/serverless/client"
	"github.com/google/trillian-examples/serverless/config"
	"github.com/transparency-dev/merkle/rfc6962"
	"golang.org/x/mod/sumdb/note"

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
		URL: u,
	}

	wg := &sync.WaitGroup{}
	for _, l := range cfg.Logs {
		wg.Add(1)
		go func(l config.Log, w wit_http.Witness) {
			defer wg.Done()
			if err := feedLog(l, witness, *timeout, *interval); err != nil {
				glog.Errorf("feedLog: %v", err)
			}
		}(l, witness)
	}
	wg.Wait()
}

func feedLog(l config.Log, w wit_http.Witness, timeout time.Duration, interval time.Duration) error {
	lURL, err := url.Parse(l.URL)
	if err != nil {
		return fmt.Errorf("invalid LogURL %q: %v", l.URL, err)
	}
	f := newFetcher(lURL)
	h := rfc6962.DefaultHasher

	fetchCP := func(ctx context.Context) ([]byte, error) {
		return f(ctx, "checkpoint")
	}
	fetchProof := func(ctx context.Context, from, to log.Checkpoint) ([][]byte, error) {
		if from.Size == 0 {
			return [][]byte{}, nil
		}
		pb, err := client.NewProofBuilder(ctx, to, h.HashChildren, f)
		if err != nil {
			return nil, fmt.Errorf("failed to create proof builder for %q: %v", l.Origin, err)
		}

		conP, err := pb.ConsistencyProof(ctx, from.Size, to.Size)
		if err != nil {
			return nil, fmt.Errorf("failed to create proof for %q(%d -> %d): %v", l.Origin, from.Size, to.Size, err)
		}
		return conP, nil
	}

	opts := feeder.FeedOpts{
		LogID:           l.ID,
		LogOrigin:       l.Origin,
		FetchCheckpoint: fetchCP,
		FetchProof:      fetchProof,
		LogSigVerifier:  mustCreateVerifier(l.PublicKey),
		Witness:         w,
	}
	if interval > 0 {
		return feeder.Run(context.Background(), interval, opts)
	}
	_, err = feeder.FeedOnce(context.Background(), opts)
	return err
}

func mustCreateVerifier(pub string) note.Verifier {
	v, err := note.NewVerifier(pub)
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

// TODO(al): factor this stuff out and share between tools:

// newFetcher creates a Fetcher for the log at the given root location.
func newFetcher(root *url.URL) client.Fetcher {
	get := getByScheme[root.Scheme]
	if get == nil {
		panic(fmt.Errorf("unsupported URL scheme %s", root.Scheme))
	}

	return func(ctx context.Context, p string) ([]byte, error) {
		u, err := root.Parse(p)
		if err != nil {
			return nil, err
		}
		return get(ctx, u)
	}
}

var getByScheme = map[string]func(context.Context, *url.URL) ([]byte, error){
	"http":  readHTTP,
	"https": readHTTP,
	"file": func(_ context.Context, u *url.URL) ([]byte, error) {
		return ioutil.ReadFile(u.Path)
	},
}

func readHTTP(ctx context.Context, u *url.URL) ([]byte, error) {
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)
}
