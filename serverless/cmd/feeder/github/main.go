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

// feeder/github is a feeder implementation for GitHub Action based serverless logs.
//
// This tool feeds checkpoints from a github-based serverless log into one or more
// witnesses, and then raises a PR against the log repo to add any resulting
//  additional witness signatures to the log's checkpoint.witnessed file.
package main

import (
	"bytes"
	"context"
	"encoding/json"
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

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/golang/glog"
	"github.com/google/go-github/v37/github"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/trillian-examples/serverless/client"
	"github.com/google/trillian-examples/serverless/cmd/feeder/impl"
	"golang.org/x/mod/sumdb/note"
	"golang.org/x/oauth2"

	wit_http "github.com/google/trillian-examples/serverless/cmd/feeder/impl/http"
)

// TODO: copied from feed-to-github; adapt as necessary.
const usage = `Usage:
 feed-to-github --log_owner_repo --witness_owner_repo --log_repo_path --feeder_config_file --interval

Where:
 --log_owner_repo is the repo owner/fragment from the repo URL.
     e.g. github.com/AlCutter/serverless-test -> AlCutter/serverless-test
 --witness_owner_repo is the repo owner/fragment of the forked log to use for the PR branch.
 --log_repo_path is the path from the root of the repo where the log files can be found,
 --feeder_config_file is the path to the config file for the serverless/cmd/feeder command.
 --interval if set, the script will continously feed and (if needed) create witness PRs sleeping
     the specified number of seconds between attempts. If not provided, the tool does a one-shot feed.

`

var (
	logOwnerRepo     = flag.String("log_owner_repo", "", "The repo owner/fragment from the log repo URL.")
	witnessOwnerRepo = flag.String("witness_owner_repo", "", "The repo owner/fragment from the witness (forked log) repo URL.")
	logRepoPath      = flag.String("log_repo_path", "", "Path from the root of the repo where the log files can be found.")
	feederConfig     = flag.String("feeder_config_file", "", "Path to the config file for the serverless/cmd/feeder command.")
	interval         = flag.Duration("interval", time.Duration(0), "Interval between checkpoints.")
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
	opts := mustValidateFlagsAndEnv()
	ctx := context.Background()

	// Make a tempdir to clone the witness (forked) log into.
	tmpDir, err := os.MkdirTemp(os.TempDir(), "feeder-github")
	if err != nil {
		glog.Exitf("Error creating temp dir: %v", err)
	}
	opts.witnessClonePath = tmpDir

	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			glog.Warning("RemoveAll err: %v", err)
		}
	}()

	witnessRepo, err := setupWitnessRepo(ctx, opts)
	if err != nil {
		glog.Exitf("Error during set-up of witness repo: %v", err)
	}

	// Authenticate to Github.
	ghClient, err := authWithGithub(ctx, opts.githubAuthToken)
	if err != nil {
		glog.Exitf("Error authenticating to Github: %v", err)
	}
	glog.V(1).Info("Created Github client")

	// Overview of the main feeder loop:
	//   1. run feeder to check if there are new signatures
	//   2. if not, sleep until next time to check.
	//   3. if so, move output from feeder onto new branch
	//   4. make commit to branch
	//   5. create checkpoint PR
	//   6. clean up
	//   7. --end of cycle--

	fmt.Println("[Starting feeding]---------------------------------------------------")
	for {
		if err = createCheckpointPR(ctx, opts, witnessRepo, ghClient); err != nil || opts.feederInterval == 0 {
			break
		}
		fmt.Println("[Feed cycle complete]------------------------------------------------")

		<-time.After(opts.feederInterval)
	}
	if err != nil {
		glog.Exitf("CreateCheckpointPR error: %v", err)
	}

	fmt.Println("[Completed feeding]--------------------------------------------------")
}

type options struct {
	gitUsername, gitEmail string
	githubAuthToken       string

	logOwnerRepo, logRepoPath string
	witnessOwnerRepo          string
	witnessClonePath          string

	feederConfigFile string
	feederInterval   time.Duration
}

func mustValidateFlagsAndEnv() *options {
	if *logOwnerRepo == "" {
		glog.Exitf("Missing required --log_owner_repo flag.\n\n%v", usage)
	}
	if *witnessOwnerRepo == "" {
		glog.Exitf("Missing required --witness_owner_repo flag.\n\n%v", usage)
	}
	if *logRepoPath == "" {
		glog.Exitf("Missing required --log_repo_path flag.\n\n%v", usage)
	}
	if *feederConfig == "" {
		glog.Exitf("Missing required --feeder_config_file flag.\n\n%v", usage)
	}

	githubAuthToken := os.Getenv("GITHUB_AUTH_TOKEN")
	if githubAuthToken == "" {
		glog.Exitf("Unauthorized: No GITHUB_AUTH_TOKEN present")
	}
	gitUsername := os.Getenv("GIT_USERNAME")
	if gitUsername == "" {
		glog.Exitf("Environment variable GIT_USERNAME is required to make commits")
	}
	gitEmail := os.Getenv("GIT_EMAIL")
	if gitEmail == "" {
		glog.Exitf("Environment variable GIT_EMAIL is required to make commits")
	}

	return &options{
		logOwnerRepo:     *logOwnerRepo,
		logRepoPath:      *logRepoPath,
		witnessOwnerRepo: *witnessOwnerRepo,
		feederConfigFile: *feederConfig,
		feederInterval:   *interval,
		githubAuthToken:  githubAuthToken,
		gitUsername:      gitUsername,
		gitEmail:         gitEmail,
	}
}

func setupWitnessRepo(ctx context.Context, opts *options) (*git.Repository, error) {
	// Clone the fork of the log, so we can go on to make a PR against it.
	//   git clone -o origin "https://${GIT_USERNAME}:${FEEDER_GITHUB_TOKEN}@github.com/${fork_repo}.git" "${temp}"
	forkLogURL := fmt.Sprintf("https://%v:%v@github.com/%v.git", opts.gitUsername, opts.githubAuthToken, opts.witnessOwnerRepo)
	forkRepo, err := git.PlainClone(opts.witnessClonePath, false, &git.CloneOptions{
		URL: forkLogURL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to clone fork repo %q: %v", forkLogURL, err)
	}
	glog.V(1).Infof("Cloned %q into %q", forkLogURL, opts.witnessClonePath)

	// Create a remote -> logOwnerRepo
	//  git remote add upstream "https://github.com/${log_repo}.git"
	logURL := fmt.Sprintf("https://github.com/%v.git", opts.logOwnerRepo)
	logRemote, err := forkRepo.CreateRemote(&config.RemoteConfig{
		Name: "upstream",
		URLs: []string{logURL},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to add remote %q to fork repo: %v", logURL, err)
	}
	glog.V(1).Infof("Added remote upstream->%q for %q: %v", logURL, opts.witnessOwnerRepo, logRemote)

	// Ensure the forkRepo config has git username and email set.
	//  git config user.name "${GIT_USERNAME}"
	//  git config user.email "${GIT_EMAIL}"
	cfg, err := forkRepo.Config()
	if err != nil {
		return nil, fmt.Errorf("failed to read config for repo %q: %v", opts.witnessOwnerRepo, err)
	}
	cfg.User.Name = opts.gitUsername
	cfg.User.Email = opts.gitEmail
	if err = forkRepo.SetConfig(cfg); err != nil {
		return nil, fmt.Errorf("failed to update config for repo %q: %v", opts.witnessOwnerRepo, err)
	}

	//  git fetch --all
	if err = forkRepo.FetchContext(ctx, &git.FetchOptions{}); err != nil && err != git.NoErrAlreadyUpToDate {
		return nil, fmt.Errorf("failed to fetch repo %q: %v", *witnessOwnerRepo, err)
	}

	return forkRepo, nil
}

func authWithGithub(ctx context.Context, ghToken string) (*github.Client, error) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: ghToken})
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc), nil
}

func pullAndGetRepoHead(r *git.Repository) (*plumbing.Reference, *git.Worktree, error) {
	// Figure out where the worktree is for repo 'r'.
	wt, err := r.Worktree()
	if err != nil {
		return nil, nil, fmt.Errorf("workTree(%v) err: %v", r, err)
	}
	// Pull the latest commits from remote 'origin'.
	if err := wt.Pull(&git.PullOptions{RemoteName: "origin"}); err != nil && err != git.NoErrAlreadyUpToDate {
		return nil, nil, fmt.Errorf("git pull %v/origin err: %v", r, err)
	}
	// Get the master HEAD - we'll need this to branch from later on.
	headRef, err := r.Head()
	if err != nil {
		return nil, nil, fmt.Errorf("reading %v HEAD ref err: %v", r, err)
	}
	// Print the latest commit that was just pulled
	commit, err := r.CommitObject(headRef.Hash())
	if err != nil {
		return nil, nil, fmt.Errorf("reading %v HEAD commit err: %v", r, err)
	}
	glog.V(1).Infof("Reading %v: HEAD commit:\n%v", r, commit)
	return headRef, wt, nil
}

func createCheckpointPR(ctx context.Context, opts *options, forkRepo *git.Repository, ghCli *github.Client) error {
	// Pull forkRepo and get the ref for origin/master branch HEAD.
	headRef, workTree, err := pullAndGetRepoHead(forkRepo)
	if err != nil {
		return err
	}

	// TODO: if checkpoint and checkpoint.witnessed bodies are different (strictly, if checkpoint is newer),
	// then we need to be feeding checkpoint to the witness, otherwise we can feed the witnessed one and
	// short-circuit creating a PR if our witness(es) has/have already signed it.
	inputCP := fmt.Sprintf("%s/checkpoint", filepath.Join(opts.witnessClonePath, opts.logRepoPath))
	glog.V(1).Infof("Reading CP from %q", inputCP)
	cp, err := ioutil.ReadFile(inputCP)
	if err != nil {
		return fmt.Errorf("failed to read input checkpoint: %v", err)
	}

	// 2. kick off feeder.
	glog.V(1).Infof("CP to feed:\n%s", string(cp))
	wCp, err := feed(ctx, cp, opts)
	if err != nil {
		return err
	}

	witnessNote := string(wCp)
	lines := strings.Split(witnessNote, "\n")
	if len(lines) < 3 {
		return fmt.Errorf("witnessed checkpoint Note seems to have too few lines: \n%v", witnessNote)
	}
	// TODO: can we use actual note struct to do this.
	cpSize, cpHash := lines[1], lines[2]

	glog.V(1).Infof("CP after feeding:\n%s", witnessNote)

	if bytes.Equal(cp, wCp) {
		fmt.Println("No signatures added")
		return nil
	}

	// Create a git branch for the witnessed checkpoint to be added, named
	// using the base64(checkpoint hash).  Construct a fully-specified
	// reference name for the new branch.
	branchRefName := plumbing.ReferenceName("refs/heads/witness_" + cpHash)
	branchHashRef := plumbing.NewHashReference(branchRefName, headRef.Hash())

	// git checkout `branchRefName`
	glog.V(1).Infof("git checkout -b %q", branchHashRef.Name())
	if err := workTree.Checkout(&git.CheckoutOptions{
		Branch: branchRefName,
		Create: true,
	}); err != nil {
		return fmt.Errorf("git checkout %v: %v", branchRefName, err)
	}
	defer func() {
		if err := forkRepo.Storer.RemoveReference(branchHashRef.Name()); err != nil {
			glog.Errorf("failed to delete branch hashref for %q: %v", branchHashRef.Name(), err)
		}
		glog.V(1).Infof("git branch -D %v", branchHashRef.Name())
		if err := workTree.Checkout(&git.CheckoutOptions{
			Branch: "refs/heads/master",
		}); err != nil {
			glog.Errorf("failed to git checkout master: %v", err)
			return
		}
		glog.V(1).Info("git checkout master")
	}()

	// 2. serialize witnessed CP to file
	repoLocalCPPath := fmt.Sprintf("%s/checkpoint.witnessed", opts.logRepoPath)
	absoluteWitnessedCPPath := filepath.Join(opts.witnessClonePath, repoLocalCPPath)
	glog.Infof("Writing witnessed CP to %q", absoluteWitnessedCPPath)
	if err := ioutil.WriteFile(absoluteWitnessedCPPath, wCp, 0644); err != nil {
		return fmt.Errorf("writing checkpoint.witnessed: %v", err)
	}

	// 3. git add the wCP file
	glog.V(1).Info("git add checkpoint.witnessed")
	if _, err := workTree.Add(repoLocalCPPath); err != nil {
		return fmt.Errorf("git add checkpoint.witnessed: %v", err)
	}

	// 4. git commit
	glog.V(1).Info("git commit")
	commit, err := workTree.Commit(fmt.Sprintf("Witness checkpoint@%v", cpSize), &git.CommitOptions{
		Author: &object.Signature{
			// Name, Email required despite what we set up in git config earlier.
			Name:  opts.gitUsername,
			Email: opts.gitEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("git commit: %v", err)
	}
	obj, err := forkRepo.CommitObject(commit)
	if err != nil {
		return fmt.Errorf("git show -s: %v", err)
	}
	glog.V(1).Infof("git show -s:\n%v", obj)

	// 5. git force-push to origin/branchrefname
	glog.V(1).Info("git push -f origin/branchrefname")
	if err := forkRepo.Push(&git.PushOptions{
		Force: true,
	}); err != nil {
		return fmt.Errorf("git push -f: %v", err)
	}

	// 6. Create GH pull request
	// 7. git checkout master  (done in defer)
	// 8. delete branch branchRefName  (done in defer)
	forkOwner := strings.Split(opts.witnessOwnerRepo, "/")[0]
	forkBranchName := strings.ReplaceAll(string(branchHashRef.Name()), "refs/heads/", "")
	return createPR(ctx, opts, ghCli, "Witness @ "+cpSize, forkOwner+":"+forkBranchName, "master")
}

// createPR creates a pull request. Based on: https://godoc.org/github.com/google/go-github/github#example-PullRequestsService-Create
func createPR(ctx context.Context, opts *options, ghCli *github.Client, title, commitBranch, prBranch string) error {
	if title == "" {
		return errors.New("missing `title`, won't create PR.")
	}

	newPR := &github.NewPullRequest{
		Title:               github.String(title),
		Head:                github.String(commitBranch),
		Base:                github.String(prBranch),
		MaintainerCanModify: github.Bool(true),
	}

	prJson, _ := json.Marshal(newPR)
	glog.V(1).Infof("Creating PR:\n%s", prJson)

	owner := strings.Split(opts.logOwnerRepo, "/")[0]
	repo := strings.Split(opts.witnessOwnerRepo, "/")[1]
	pr, _, err := ghCli.PullRequests.Create(ctx, owner, repo, newPR)
	if err != nil {
		return err
	}

	glog.Infof("PR created: %s", pr.GetHTMLURL())
	return nil
}

func feed(ctx context.Context, cp []byte, opts *options) ([]byte, error) {
	cfg, err := readConfig(opts.feederConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read feeder config file: %v", err)
	}

	lURL, err := url.Parse(cfg.LogURL)
	if err != nil {
		return nil, fmt.Errorf("invalid LogURL %q: %v", cfg.LogURL, err)
	}

	logSigV, err := note.NewVerifier(cfg.LogPublicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid log public key: %v", err)
	}

	fOpts := impl.FeedOpts{
		LogID:          cfg.LogID,
		LogFetcher:     newFetcher(lURL),
		LogSigVerifier: logSigV,
		NumRequired:    cfg.NumRequired,
		WitnessTimeout: 5 * time.Second,
	}
	for _, w := range cfg.Witnesses {
		u, err := url.Parse(w.URL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse witness URL %q: %v", w.URL, err)
		}
		wSigV, err := note.NewVerifier(w.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("invalid witness public key for url %q: %v", w.URL, err)
		}
		fOpts.Witnesses = append(fOpts.Witnesses, wit_http.Witness{
			URL:      u,
			Verifier: wSigV,
		})
	}

	wCP, err := impl.Feed(ctx, cp, fOpts)
	if err != nil {
		return nil, fmt.Errorf("feeding failed: %v", err)
	}

	return wCP, nil
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
