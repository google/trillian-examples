// Copyright 2018 Google Inc. All Rights Reserved.
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

package integration_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/google/trillian-examples/gossip/hub"
	"github.com/google/trillian-examples/gossip/integration"
	"github.com/google/trillian/storage/testdb"
)

var (
	hubServers  = flag.String("hub_servers", "", "Comma-separated list of (assumed interchangeable) hub servers, each as address:port")
	hubConfig   = flag.String("hub_config", "", "File holding Hub configuration")
	srcPrivKeys = flag.String("src_priv_keys", "", "List of files holding source private keys (comma-separated); must match --hub_config")
	hubCount    = flag.Int("hubs", 3, "Number of parallel Gossip Hubs to test")
	logCount    = flag.Int("logs", 4, "Number of source logs tracked by each hub")
	mmd         = flag.Duration("mmd", 120*time.Second, "MMD for tested hubs")
)

func TestInProcessGossipIntegration(t *testing.T) {
	testdb.SkipIfNoMySQL(t)
	ctx := context.Background()
	cfgs, logKeys, err := integration.BuildTestConfig(*hubCount, *logCount)
	if err != nil {
		t.Fatalf("Failed to build test config: %v", err)
	}
	env, err := integration.NewHubEnv(ctx, cfgs, 2, "TestInProcessCTIntegration")
	if err != nil {
		t.Fatalf("Failed to launch test environment: %v", err)
	}
	defer env.Close()

	// Run a container for the parallel sub-tests, so that we wait until they
	// all complete before terminating the test environment.
	t.Run("container", func(t *testing.T) {
		for _, cfg := range cfgs {
			cfg := cfg // capture config
			t.Run(cfg.Prefix, func(t *testing.T) {
				t.Parallel()
				if err := integration.RunIntegrationForHub(ctx, cfg, env.HubAddr, *mmd, logKeys); err != nil {
					t.Errorf("%s: failed: %v", cfg.Prefix, err)
				}
			})
		}
	})
}

func TestLiveGossipIntegration(t *testing.T) {
	flag.Parse()
	if *hubServers == "" {
		t.Skip("Integration test skipped as no hub server addresses provided")
	}
	if *hubConfig == "" {
		t.Skip("Integration test skipped as no hub configuration provided")
	}
	if *srcPrivKeys == "" {
		t.Skip("Integration test skipped as no source private keys provided")
	}
	ctx := context.Background()

	multiCfg, err := hub.ConfigFromSingleFile(*hubConfig, "placeholder")
	if err != nil {
		t.Fatalf("Failed to read Hub config file: %v", err)
	}

	var logKeys []*ecdsa.PrivateKey
	for _, filename := range strings.Split(*srcPrivKeys, ",") {
		logKey, err := privateKeyFromPEM(filename)
		if err != nil {
			t.Fatalf("Failed to read private key %q: %v", filename, err)
		}
		logKeys = append(logKeys, logKey)
	}

	for _, cfg := range multiCfg.HubConfig {
		cfg := cfg // capture config
		t.Run(cfg.Prefix, func(t *testing.T) {
			t.Parallel()
			if err := integration.RunIntegrationForHub(ctx, cfg, *hubServers, *mmd, logKeys); err != nil {
				t.Errorf("%s: failed: %v", cfg.Prefix, err)
			}
		})
	}
}

func privateKeyFromPEM(filename string) (*ecdsa.PrivateKey, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file: %v", err)
	}
	block, _ := pem.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode PEM file: %v", err)
	}
	privKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ECDSA key: %v", err)
	}
	return privKey, nil
}
