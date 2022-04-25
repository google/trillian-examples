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

// distribute/github redistributes witnessed checkpoints from one or more logs,
// fetched from a single witness, to a single serverless distributor.
package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/golang/glog"

	"github.com/google/trillian-examples/internal/github"
	"github.com/google/trillian-examples/serverless/config"
	"golang.org/x/mod/sumdb/note"

	dist_gh "github.com/google/trillian-examples/internal/distribute/github"
	i_note "github.com/google/trillian-examples/internal/note"
	wit_http "github.com/google/trillian-examples/witness/golang/client/http"
	yaml "gopkg.in/yaml.v2"
)

const usage = `Usage:
 distribute --distributor_repo --fork_repo --distributor_path --config_file --interval

Where:
 --distributor_repo is the repo owner/fragment from the distributor github repo URL.
     e.g. github.com/AlCutter/serverless-test -> AlCutter/serverless-test
 --distributor_branch is the name of the primary branch on the distributor repo (e.g. main).
 --fork_repo is the repo owner/fragment of the forked distributor to use for the PR branch.
 --distributor_path is the path from the root of the repo where the distributor files can be found,
 --config_file is the path to the config file for the serverless/cmd/feeder command.
 --interval if set, the script will continuously feed and (if needed) create witness PRs sleeping
     the specified number of seconds between attempts. If not provided, the tool does a one-shot feed.

`

var (
	distributorRepo   = flag.String("distributor_repo", "", "The repo owner/fragment from the distributor repo URL.")
	distributorBranch = flag.String("distributor_branch", "master", "The branch that PRs will be proposed against on the distributor_repo.")
	forkRepo          = flag.String("fork_repo", "", "The repo owner/fragment from the feeder (forked distributor) repo URL.")
	distributorPath   = flag.String("distributor_path", "", "Path from the root of the repo where the distributor files can be found.")
	witSigV           = flag.String("witness_vkey", "", "Witness public key.")
	configPath        = flag.String("config_file", "", "Path to the config file for the serverless/cmd/feeder command.")
	interval          = flag.Duration("interval", time.Duration(0), "Interval between checkpoints. Default of 0 causes the tool to be a one-shot.")
)

func main() {
	flag.Parse()
	ctx := context.Background()

	opts := mustConfigure(ctx, http.DefaultClient)

	if err := dist_gh.DistributeOnce(ctx, opts); err != nil {
		glog.Warningf("DistributeOnce: %v", err)
	}
	if *interval > 0 {
		for {
			select {
			case <-time.After(*interval):
				glog.V(1).Infof("Wait time is up, going around again (%s)", *interval)
			case <-ctx.Done():
				return
			}
			if err := dist_gh.DistributeOnce(ctx, opts); err != nil {
				glog.Warningf("DistributeOnce: %v", err)
			}
		}
	}
}

// usageExit prints out a message followed by the usage string, and then terminates execution.
func usageExit(m string) {
	glog.Exitf("%s\n\n%s", m, usage)
}

// mustConfigure creates an options struct from flags and env vars.
// It will terminate execution on any error.
func mustConfigure(ctx context.Context, c *http.Client) *dist_gh.DistributeOptions {
	checkNotEmpty := func(m, v string) {
		if v == "" {
			usageExit(m)
		}
	}
	// Check flags
	checkNotEmpty("Missing required --distributor_repo flag", *distributorRepo)
	checkNotEmpty("Missing required --fork_repo flag", *forkRepo)
	checkNotEmpty("Missing required --distributor_path flag", *distributorPath)
	checkNotEmpty("Missing required --config_file flag", *configPath)
	checkNotEmpty("Missing required --witness_vkey flag", *witSigV)

	// Check env vars
	githubAuthToken := os.Getenv("GITHUB_AUTH_TOKEN")
	gitUsername := os.Getenv("GIT_USERNAME")
	gitEmail := os.Getenv("GIT_EMAIL")

	checkNotEmpty("Unauthorized: No GITHUB_AUTH_TOKEN present", githubAuthToken)
	checkNotEmpty("Environment variable GIT_USERNAME is required to make commits", gitUsername)
	checkNotEmpty("Environment variable GIT_EMAIL is required to make commits", gitEmail)

	dr, err := github.NewRepoID(*distributorRepo)
	if err != nil {
		usageExit(fmt.Sprintf("--distributor_repo invalid: %v", err))
	}
	fr, err := github.NewRepoID(*forkRepo)
	if err != nil {
		usageExit(fmt.Sprintf("--fork_repo invalid: %v", err))
	}

	cfg, err := readConfig(*configPath)
	if err != nil {
		glog.Exitf("Feeder config in %q is invalid: %v", *configPath, err)
	}

	u, err := url.Parse(cfg.Witness.URL)
	if err != nil {
		glog.Exitf("Failed to parse witness URL %q: %v", cfg.Witness.URL, err)
	}
	wSigV, err := note.NewVerifier(*witSigV)
	if err != nil {
		glog.Exitf("Invalid witness public key for url %q: %v", cfg.Witness.URL, err)
	}

	logs := make([]dist_gh.Log, 0)

	for _, l := range cfg.Logs {
		log := dist_gh.Log{
			Config: l,
		}
		lSigV, err := i_note.NewVerifier(l.PublicKeyType, l.PublicKey)
		if err != nil {
			glog.Exitf("Invalid log public key: %v", err)
		}
		log.SigV = lSigV
		logs = append(logs, log)
	}

	repo, err := github.NewRepository(ctx, c, dr, *distributorBranch, fr, gitUsername, gitEmail, githubAuthToken)
	if err != nil {
		glog.Exitf("Failed to set up repository: %v", err)
	}

	return &dist_gh.DistributeOptions{
		Repo:            repo,
		DistributorPath: *distributorPath,
		Logs:            logs,
		WitSigV:         wSigV,
		Witness:         wit_http.NewWitness(u, http.DefaultClient),
	}
}

// distributeConfig is the format of this tool's config file.
type distributeConfig struct {
	// Log defines the log checkpoints are being distributed for.
	Logs []config.Log `yaml:"Logs"`

	// Witness defines the witness to read from.
	Witness config.Witness `yaml:"Witness"`
}

// Validate checks that the config is populated correctly.
func (c distributeConfig) Validate() error {
	for _, l := range c.Logs {
		if err := l.Validate(); err != nil {
			return err
		}
	}
	return c.Witness.Validate()
}

// readConfig parses the named file into a FeedOpts structure.
func readConfig(f string) (distributeConfig, error) {
	cfg := distributeConfig{}
	c, err := ioutil.ReadFile(f)
	if err != nil {
		return cfg, fmt.Errorf("failed to read file: %v", err)
	}
	if err := yaml.Unmarshal(c, &cfg); err != nil {
		return cfg, fmt.Errorf("failed to unmarshal config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("invalid config at %q: %v", f, err)
	}
	return cfg, nil
}
