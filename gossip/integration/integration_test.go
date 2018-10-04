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
	"flag"
	"testing"
	"time"

	"github.com/google/trillian-examples/gossip/integration"
	"github.com/google/trillian/storage/testdb"
)

var (
	hubCount = flag.Int("hubs", 3, "Number of parallel Gossip Hubs to test")
	logCount = flag.Int("logs", 3, "Number of source logs tracked by each hub")
	mmd      = flag.Duration("mmd", 120*time.Second, "MMD for tested hubs")
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
