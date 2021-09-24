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
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/glog"
	"gopkg.in/yaml.v2"

	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/serverless/client"
	"github.com/google/trillian-examples/serverless/cmd/feeder/impl"
	"github.com/google/trillian-examples/serverless/config"
	"github.com/google/trillian-examples/serverless/internal/github"
	"golang.org/x/mod/sumdb/note"

	wit_http "github.com/google/trillian-examples/witness/golang/client/http"
)

// TODO: copied from feed-to-github; adapt as necessary.
const usage = `Usage:
 feed-to-github --distributor_owner_repo --feeder_owner_repo --distributor_repo_path --feeder_config_file --interval

Where:
 --distributor_owner_repo is the repo owner/fragment from the distributor github repo URL.
     e.g. github.com/AlCutter/serverless-test -> AlCutter/serverless-test
 --feeder_owner_repo is the repo owner/fragment of the forked distributor to use for the PR branch.
 --distributor_repo_path is the path from the root of the repo where the distributor files can be found,
 --feeder_config_file is the path to the config file for the serverless/cmd/feeder command.
 --interval if set, the script will continuously feed and (if needed) create witness PRs sleeping
     the specified number of seconds between attempts. If not provided, the tool does a one-shot feed.

`

var (
	distributorOwnerRepo = flag.String("distributor_owner_repo", "", "The repo owner/fragment from the distributor repo URL.")
	distributorBranch    = flag.String("distributor_branch", "master", "The branch that PRs will be proposed against on the distributor_repo.")
	feederOwnerRepo      = flag.String("feeder_owner_repo", "", "The repo owner/fragment from the feeder (forked distributor) repo URL.")
	distributorRepoPath  = flag.String("distributor_repo_path", "", "Path from the root of the repo where the distributor files can be found.")
	feederConfigPath     = flag.String("feeder_config_file", "", "Path to the config file for the serverless/cmd/feeder command.")
	interval             = flag.Duration("interval", time.Duration(0), "Interval between checkpoints.")
	cloneToDisk          = flag.Bool("clone_to_disk", false, "Whether to clone the distributor repo to memory or disk.")
)

func main() {
	flag.Parse()
	opts := mustConfigure()
	ctx := context.Background()

	if *cloneToDisk {
		// Make a tempdir to clone the witness (forked) distributor into.
		tmpDir, err := os.MkdirTemp(os.TempDir(), "feeder-github")
		if err != nil {
			glog.Exitf("Error creating temp dir: %v", err)
		}
		opts.feederClonePath = tmpDir
		defer func() {
			if err := os.RemoveAll(tmpDir); err != nil {
				glog.Warningf("RemoveAll err: %v", err)
			}
		}()
	}

	repo, err := github.NewRepository(ctx, opts.distributorRepo, *distributorBranch, opts.forkRepo, opts.gitUsername, opts.gitEmail, opts.githubAuthToken, opts.feederClonePath)
	if err != nil {
		glog.Exitf("Failed to set up repository: %v", err)
	}

	// Overview of the main feeder loop:
	//   1. run feeder to check if there are new signatures
	//   2. if not, sleep until next time to check.
	//   3. if so, move output from feeder onto new branch
	//   4. make commit to branch
	//   5. create checkpoint PR
	//   6. clean up
	//   7. --end of cycle--

	glog.Info("[Starting feeding]---------------------------------------------------")
	delay := time.Duration(0)
mainLoop:
	for {
		select {
		case <-time.After(delay):
			delay = opts.feedInterval
		case <-ctx.Done():
			break mainLoop
		}

		if err := feedOnce(ctx, opts, repo); err != nil {
			glog.Warningf("feedOnce: %v", err)
			continue
		}
		glog.Info("[Feed cycle complete]------------------------------------------------")
	}

	glog.Info("[Completed feeding]--------------------------------------------------")
}

// feedOnce performs a one-shot "feed to witness and create PR" operation.
func feedOnce(ctx context.Context, opts *options, repo github.Repository) error {
	// Pull forkRepo and get the ref for origin/master branch HEAD.
	headRef, err := repo.PullAndGetHead()
	if err != nil {
		return err
	}

	cpRaw, err := selectCPToFeed(ctx, repo, opts)
	if err != nil {
		return err
	}

	glog.V(1).Infof("Feeding CP:\n%s", string(cpRaw))
	wRaw, err := impl.Feed(ctx, cpRaw, opts.feederOpts)
	if err != nil {
		if err != impl.ErrNoSignaturesAdded {
			return err
		}
		glog.Info("No new witness signatures added")
		return nil
	}
	glog.V(1).Infof("Witnessed CP:\n%s", string(wRaw))

	wCp, _, witnessNote, err := log.ParseCheckpoint(wRaw, opts.feederOpts.LogOrigin, opts.feederOpts.LogSigVerifier, opts.witSigV)
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

	outputPath := filepath.Join(opts.distributorPath, "logs", opts.feederOpts.LogID, "incoming", fmt.Sprintf("checkpoint_%s", id))
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

// selectCPToFeed decides which checkpoint, if any, to attempt to feed to the witness.
func selectCPToFeed(ctx context.Context, repo github.Repository, opts *options) ([]byte, error) {
	logCPRaw, err := opts.feederOpts.LogFetcher(ctx, "checkpoint")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch log checkpoint: %v", err)
	}
	logCP, _, _, err := log.ParseCheckpoint(logCPRaw, opts.feederOpts.LogOrigin, opts.feederOpts.LogSigVerifier)
	if err != nil {
		return nil, fmt.Errorf("failed to parse log checkpoint: %v", err)
	}
	cpZeroPath := filepath.Join(opts.distributorPath, "logs", opts.feederOpts.LogID, "checkpoint.0")
	cpZeroRaw, err := repo.ReadFile(cpZeroPath)
	if err != nil {
		glog.Warningf("Failed to read %q: %v. Assuming new distributor and continuing...", cpZeroPath, err)
		return logCPRaw, nil
	}
	cpZero, _, n, err := log.ParseCheckpoint(cpZeroRaw, opts.feederOpts.LogOrigin, opts.feederOpts.LogSigVerifier, opts.feederOpts.Witness.SigVerifier())
	if err != nil {
		return nil, fmt.Errorf("failed to parse checkpoint.0: %v", err)
	}

	if logCP.Size > cpZero.Size {
		// Log checkpoint is newer, witness that.
		return logCPRaw, nil
	}
	if logCP.Size < cpZero.Size {
		// Could be a caching thing, warn but we'll continue...
		glog.Warningf("Whoa, the distributor has a larger checkpoint (%d) than the log (%d):\n%s\n%s", cpZero.Size, logCP.Size, string(cpZeroRaw), string(logCPRaw))
	}

	if len(n.Sigs) > 1 {
		// We're done - we've already signed this CP.
		return nil, fmt.Errorf("nothing to do - largest CP is already witnessed by %s", opts.feederOpts.Witness.SigVerifier().Name())
	}
	return cpZeroRaw, nil
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
	feederClonePath string

	witSigV note.Verifier

	feederOpts   impl.FeedOpts
	feedInterval time.Duration
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
	checkNotEmpty("Missing required --distributor_owner_repo flag", *distributorOwnerRepo)
	checkNotEmpty("Missing required --feeder_owner_repo flag", *feederOwnerRepo)
	checkNotEmpty("Missing required --distributor_repo_path flag", *distributorRepoPath)
	checkNotEmpty("Missing required --feeder_config_file flag", *feederConfigPath)

	// Check env vars
	githubAuthToken := os.Getenv("GITHUB_AUTH_TOKEN")
	gitUsername := os.Getenv("GIT_USERNAME")
	gitEmail := os.Getenv("GIT_EMAIL")

	checkNotEmpty("Unauthorized: No GITHUB_AUTH_TOKEN present", githubAuthToken)
	checkNotEmpty("Environment variable GIT_USERNAME is required to make commits", gitUsername)
	checkNotEmpty("Environment variable GIT_EMAIL is required to make commits", gitEmail)

	dr, err := github.NewRepoID(*distributorOwnerRepo)
	if err != nil {
		usageExit(fmt.Sprintf("--distributor_owner_repo invalid: %v", err))
	}
	fr, err := github.NewRepoID(*feederOwnerRepo)
	if err != nil {
		usageExit(fmt.Sprintf("--feeder_owner_repo invalid: %v", err))
	}

	fOpts, err := readFeederConfig(*feederConfigPath)
	if err != nil {
		glog.Exitf("Feeder config in %q is invalid: %v", *feederConfigPath, err)
	}

	return &options{
		distributorRepo: dr,
		forkRepo:        fr,
		distributorPath: *distributorRepoPath,
		githubAuthToken: githubAuthToken,
		gitUsername:     gitUsername,
		gitEmail:        gitEmail,
		witSigV:         fOpts.Witness.SigVerifier(),
		feederOpts:      *fOpts,
		feedInterval:    *interval,
	}
}

// feederConfig is the format of the feeder config file.
type feederConfig struct {
	Log config.Log `yaml:"Log"`

	// Witness defines the target witness
	Witness config.Witness `yaml:"Witness"`
}

// readFeederConfig parses the named file into a FeedOpts structure.
func readFeederConfig(f string) (*impl.FeedOpts, error) {
	c, err := ioutil.ReadFile(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}
	cfg := &feederConfig{}
	if err := yaml.Unmarshal(c, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %v", err)
	}

	lURL, err := url.Parse(cfg.Log.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid LogURL %q: %v", cfg.Log.URL, err)
	}

	logSigV, err := note.NewVerifier(cfg.Log.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid log public key: %v", err)
	}

	fOpts := impl.FeedOpts{
		LogID:          cfg.Log.ID,
		LogFetcher:     newFetcher(lURL),
		LogSigVerifier: logSigV,
		LogOrigin:      cfg.Log.Origin,
		WitnessTimeout: 5 * time.Second,
	}

	u, err := url.Parse(cfg.Witness.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse witness URL %q: %v", cfg.Witness.URL, err)
	}
	wSigV, err := note.NewVerifier(cfg.Witness.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid witness public key for url %q: %v", cfg.Witness.URL, err)
	}
	fOpts.Witness = wit_http.Witness{
		URL:      u,
		Verifier: wSigV,
	}

	return &fOpts, nil
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
