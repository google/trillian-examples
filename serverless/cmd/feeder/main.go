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

// feeder is a one-shot witness feeder implementation for the serverless log.
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
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/serverless/client"
	"github.com/google/trillian-examples/serverless/cmd/feeder/impl"
	"golang.org/x/mod/sumdb/note"

	wit_http "github.com/google/trillian-examples/witness/golang/client/http"
)

var (
	configFile = flag.String("config_file", "", "Path to feeder config file.")
	input      = flag.String("input", "", "Path/HTTP URL to input checkpoint file, leave empty for stdin")
	output     = flag.String("output", "", "Path to write cosigned checkpoint to, leave empty for stdout")
	timeout    = flag.Duration("timeout", 10*time.Second, "Maximum time to wait for witnesses to respond.")
)

type WitnessConfig struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	PublicKey string `json:"public_key"`
}

// Config encapsulates the feeder config.
type Config struct {
	// The LogID used by the witnesses to identify this log.
	LogID string `json:"log_id"`
	// PublicKey associated with LogID.
	LogPublicKey string `json:"log_public_key"`
	// LogURL is the URL of the root of the log.
	LogURL string `json:"log_url"`
	// Witnesses is a list of all configured witnesses.
	Witnesses []WitnessConfig `json:"witnesses"`
	// NumRequired is the minimum number of cosignatures required for a feeding run
	// to be considered successful.
	NumRequired int `json:"num_required"`
}

func main() {
	flag.Parse()
	ctx, c := context.WithTimeout(context.Background(), *timeout)
	defer c()

	cfg, err := readConfig(*configFile)
	if err != nil {
		glog.Exitf("Failed to read config: %v", err)
	}

	lURL, err := url.Parse(cfg.LogURL)
	if err != nil {
		glog.Exitf("Invalid LogURL %q: %v", cfg.LogURL, err)
	}
	f := newFetcher(lURL)

	opts := impl.FeedOpts{
		LogID:          cfg.LogID,
		LogFetcher:     f,
		LogSigVerifier: mustCreateVerifier(cfg.LogPublicKey),
		NumRequired:    cfg.NumRequired,
	}
	for _, w := range cfg.Witnesses {
		u, err := url.Parse(w.URL)
		if err != nil {
			glog.Exitf("Failed to parse witness URL %q: %v", w.URL, err)
		}
		opts.Witnesses = append(opts.Witnesses, wit_http.Witness{
			URL:      u,
			Verifier: mustCreateVerifier(w.PublicKey),
		})
	}

	cp, err := readCP(ctx, *input)
	if err != nil {
		glog.Exitf("Failed to read input checkpoint: %v", err)
	}

	glog.Infof("CP to feed:\n%s", string(cp))

	wCP, err := impl.Feed(ctx, cp, opts)
	if err != nil {
		glog.Exitf("Feeding failed: %v", err)
	}

	if err := writeCP(wCP, *output); err != nil {
		glog.Exitf("Failed to write witnessed checkpoint: %v", err)
	}
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
	if err := json.Unmarshal(c, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %v", err)
	}
	return &cfg, nil
}

func readCP(ctx context.Context, f string) ([]byte, error) {
	if strings.HasPrefix(f, "http://") || strings.HasPrefix(f, "https://") {
		u, err := url.Parse(f)
		if err != nil {
			return nil, fmt.Errorf("failed to parse checkpoint URL %q: %v", f, err)
		}
		return readHTTP(ctx, u)
	}
	var from *os.File
	if f == "" {
		from = os.Stdin
	} else {
		from, err := os.Open(f)
		if err != nil {
			return nil, fmt.Errorf("failed to open input file %q: %v", f, err)
		}
		defer from.Close()
	}
	return ioutil.ReadAll(from)
}

func writeCP(cp []byte, f string) error {
	if f == "" {
		fmt.Println(string(cp))
		return nil
	}
	return ioutil.WriteFile(f, cp, 0644)
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
