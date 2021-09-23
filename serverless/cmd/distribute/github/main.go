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

// feeder/github is a feeder implementation for GitHub Action based serverless distributors.
//
// This tool feeds checkpoints from a log into one or more witnesses, and then
// raises a PR against the distributor repo to add any resulting additional
// witness signatures.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/glog"
	"gopkg.in/yaml.v2"

	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/serverless/cmd/feeder/impl"
	"github.com/google/trillian-examples/serverless/config"
	"github.com/google/trillian-examples/serverless/internal/github"
	"golang.org/x/mod/sumdb/note"

	wit_http "github.com/google/trillian-examples/witness/golang/client/http"
)

const usage = `Usage:
 distribute --distributor_repo --fork_repo --distributor_path --config_file --interval

Where:
 --distributor_repo is the repo owner/fragment from the distributor github repo URL.
     e.g. github.com/AlCutter/serverless-test -> AlCutter/serverless-test
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
	configPath        = flag.String("config_file", "", "Path to the config file for the serverless/cmd/feeder command.")
	interval          = flag.Duration("interval", time.Duration(0), "Interval between checkpoints.")
	cloneToDisk       = flag.Bool("clone_to_disk", false, "Whether to clone the distributor repo to memory or disk.")
)

func main() {
	flag.Parse()
	opts := mustConfigure()
	ctx := context.Background()

	if *cloneToDisk {
		// Make a tempdir to clone the witness (forked) distributor into.
		tmpDir, err := os.MkdirTemp(os.TempDir(), "distribute-github")
		if err != nil {
			glog.Exitf("Error creating temp dir: %v", err)
		}
		opts.clonePath = tmpDir
		defer func() {
			if err := os.RemoveAll(tmpDir); err != nil {
				glog.Warningf("RemoveAll err: %v", err)
			}
		}()
	}

	repo, err := github.NewRepository(ctx, opts.distributorRepo, *distributorBranch, opts.forkRepo, opts.gitUsername, opts.gitEmail, opts.githubAuthToken, opts.clonePath)
	if err != nil {
		glog.Exitf("Failed to set up repository: %v", err)
	}

	if err := distributeOnce(ctx, opts, repo); err != nil {
		glog.Warningf("distributeOnce: %v", err)
	}
	if *interval > 0 {
		for {
			select {
			case <-time.After(*interval):
				glog.Infof("Wait time is up, going around again (%s)", *interval)
			case <-ctx.Done():
				return
			}
			if err := distributeOnce(ctx, opts, repo); err != nil {
				glog.Warningf("distributeOnce: %v", err)
			}
		}
	}
}

// distributeOnce a) polls the witness b) updates the fork c) proposes a PR if needed.
func distributeOnce(ctx context.Context, opts *options, repo github.Repository) error {
	// Pull forkRepo and get the ref for origin/master branch HEAD.
	headRef, err := repo.PullAndGetHead()
	if err != nil {
		return err
	}

	// This will be used on both the witness and the distributor.
	// At the moment the ID is arbitrary and is up to the discretion of the operators
	// of these parties. We should address this. If we don't manage to do so in time,
	// we'll need to allow this ID to be configured separately for each entity.
	logID := opts.distConfig.Log.ID

	wRaw, err := opts.witness.GetLatestCheckpoint(ctx, logID)
	if err != nil {
		return err
	}
	wCp, _, witnessNote, err := log.ParseCheckpoint(wRaw, opts.distConfig.Log.Origin, opts.logSigV, opts.witSigV)
	if err != nil {
		return fmt.Errorf("couldn't parse witnessed checkpoint: %v", err)
	}
	if nWitSigs, want := len(witnessNote.Sigs)-1, 1; nWitSigs != want {
		return fmt.Errorf("checkpoint has %d witness sigs, want %d", nWitSigs, want)
	}

	// Now form a PR with the cosigned CP.
	id := wcpID(*witnessNote)
	branchName := fmt.Sprintf("refs/heads/witness_%s", id)
	deleteBranch, err := repo.CreateLocalBranch(headRef, branchName)
	if err != nil {
		return fmt.Errorf("failed to create git branch for PR: %v", err)
	}
	defer deleteBranch()

	outputPath := filepath.Join(opts.distributorPath, "logs", logID, "incoming", fmt.Sprintf("checkpoint_%s", id))
	// First, check whether we've already managed to submit this CP into the incoming directory
	if _, err := repo.ReadFile(outputPath); err == nil {
		return fmt.Errorf("witnessed checkpoint already pending: %v", impl.ErrNoSignaturesAdded)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to check for existing pending checkpoint: %v", err)
	}

	msg := fmt.Sprintf("Witness checkpoint@%v", wCp.Size)
	if err := repo.CommitFile(outputPath, wRaw, msg); err != nil {
		return fmt.Errorf("failed to commit updated checkpoint.witnessed file: %v", err)
	}

	if err := repo.Push(); err != nil {
		return fmt.Errorf("failed to push: %v", err)
	}

	glog.V(1).Info("Creating PR")
	return repo.CreatePR(ctx, fmt.Sprintf("Witness @ %d", wCp.Size), branchName)
}

// cpID returns a stable identifier for a given checkpoint and list of known signatures.
// This is used as a branch name, and for generating a file name for the witness PR.
func wcpID(n note.Note) string {
	parts := []string{n.Text}
	// Note that n.Sigs is sorted by checkpoints.Combine, called by feeder.Feed
	for _, s := range n.Sigs {
		parts = append(parts, fmt.Sprintf("%x", s.Hash))
	}
	h := sha256.Sum256([]byte(strings.Join(parts, "-")))
	return hex.EncodeToString(h[:])

}

// options contains the various configuration and state required to perform a feedOnce call.
type options struct {
	gitUsername, gitEmail string
	githubAuthToken       string

	forkRepo        github.RepoID
	distributorRepo github.RepoID
	distributorPath string
	clonePath       string

	distConfig distributeConfig
	logSigV    note.Verifier
	witSigV    note.Verifier
	witness    wit_http.Witness
	interval   time.Duration
}

// usageExit prints out a message followed by the usage string, and then terminates execution.
func usageExit(m string) {
	glog.Exitf("%s\n\n%s", m, usage)
}

// mustConfigure creates an options struct from flags and env vars.
// It will terminate execution on any error.
func mustConfigure() *options {
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
		glog.Exitf("failed to parse witness URL %q: %v", cfg.Witness.URL, err)
	}
	wSigV, err := note.NewVerifier(cfg.Witness.PublicKey)
	if err != nil {
		glog.Exitf("invalid witness public key for url %q: %v", cfg.Witness.URL, err)
	}

	lSigV, err := note.NewVerifier(cfg.Log.PublicKey)
	if err != nil {
		glog.Exitf("invalid log public key: %v", err)
	}

	return &options{
		distributorRepo: dr,
		forkRepo:        fr,
		distributorPath: *distributorPath,
		githubAuthToken: githubAuthToken,
		gitUsername:     gitUsername,
		gitEmail:        gitEmail,
		logSigV:         lSigV,
		witSigV:         wSigV,
		witness: wit_http.Witness{
			URL:      u,
			Verifier: wSigV,
		},
		interval:   *interval,
		distConfig: cfg,
	}
}

// distributeConfig is the format of this tool's config file.
type distributeConfig struct {
	// Log defines the log checkpoints are being distributed for.
	Log config.Log `yaml:"Log"`

	// Witness defines the witness to read from.
	Witness config.Witness `yaml:"Witness"`
}

// Validate checks that the config is populated correctly.
func (c distributeConfig) Validate() error {
	if err := c.Log.Validate(); err != nil {
		return err
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
