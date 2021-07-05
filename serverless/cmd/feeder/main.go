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

// feeder is a witness feeder implementation for the serverless log.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/serverless/client"
	"github.com/google/trillian-examples/serverless/cmd/feeder/impl"
)

var (
	configFile = flag.String("config_file", "", "Path to feeder config file.")
	input      = flag.String("input", "", "Path to input checkpoint file, leave empty for stdin")
	output     = flag.String("output", "", "Path to write cosigned checkpoint to, leave empty for stdout")
)

func main() {
	flag.Parse()
	ctx := context.Background()

	cfg, err := readConfig(*configFile)
	if err != nil {
		glog.Exitf("Failed to read config: %v", err)
	}

	lURL, err := url.Parse(cfg.LogURL)
	if err != nil {
		glog.Exitf("Invalid LogURL %q: %v", cfg.LogURL, err)
	}
	f := newFetcher(lURL)

	cp, err := readCP(*input)
	if err != nil {
		glog.Exitf("Failed to read input checkpoint: %v", err)
	}

	wCP, err := impl.Witness(ctx, cp, f, *cfg)
	if err != nil {
		glog.Exitf("Feeding failed: %v", err)
	}

	if err := writeCP(wCP, *output); err != nil {
		glog.Exitf("Failed to write witnessed checkpoint: %v", err)
	}
}

func readConfig(f string) (*impl.Config, error) {
	c, err := os.ReadFile(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}
	cfg := impl.Config{}
	if err := json.Unmarshal(c, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %v", err)
	}
	return &cfg, nil
}

func readCP(f string) ([]byte, error) {
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
	return io.ReadAll(from)
}

func writeCP(cp []byte, f string) error {
	if f == "" {
		fmt.Println(string(cp))
		return nil
	}
	return os.WriteFile(f, cp, 0644)
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
