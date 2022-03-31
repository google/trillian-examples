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
	"github.com/google/trillian-examples/internal/feeder/serverless"
	"github.com/google/trillian-examples/serverless/config"

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
	witness := wit_http.NewWitness(u, http.DefaultClient)

	ctx := context.Background()
	wg := &sync.WaitGroup{}
	for _, l := range cfg.Logs {
		wg.Add(1)
		go func(l config.Log, w wit_http.Witness) {
			defer wg.Done()

			c := &http.Client{
				Timeout: *timeout,
			}
			if err := serverless.FeedLog(ctx, l, witness, c, *interval); err != nil {
				glog.Errorf("feedLog: %v", err)
			}
		}(l, witness)
	}
	wg.Wait()
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
