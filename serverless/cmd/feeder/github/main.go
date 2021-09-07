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

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/util"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/golang/glog"
	"github.com/google/go-github/v37/github"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/serverless/client"
	"github.com/google/trillian-examples/serverless/cmd/feeder/impl"
	"golang.org/x/mod/sumdb/note"
	"golang.org/x/oauth2"

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

	feederRepo, err := setupFeederRepo(ctx, opts)
	if err != nil {
		glog.Exitf("Error during set-up of feeder repo: %v", err)
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

		if err := feedOnce(ctx, opts, feederRepo, ghClient); err != nil {
			glog.Warningf("feedOnce: %v", err)
			continue
		}
		glog.Info("[Feed cycle complete]------------------------------------------------")
	}

	glog.Info("[Completed feeding]--------------------------------------------------")
}

// feedOnce performs a one-shot "feed to witness and create PR" operation.
func feedOnce(ctx context.Context, opts *options, forkRepo *git.Repository, ghCli *github.Client) error {
	// Pull forkRepo and get the ref for origin/master branch HEAD.
	headRef, workTree, err := pullAndGetRepoHead(forkRepo)
	if err != nil {
		return err
	}

	cpRaw, err := opts.feederOpts.LogFetcher(ctx, "checkpoint")
	if err != nil {
		return fmt.Errorf("failed to fetch input checkpoint: %v", err)
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

	// Check the witness signed a log statement.
	_, err = note.Open(wRaw, note.VerifierList(opts.feederOpts.LogSigVerifier))
	if err != nil {
		return fmt.Errorf("couldn't open witnessed checkpoint with log verifier: %v", err)
	}
	witnessNote, err := note.Open(wRaw, note.VerifierList(opts.witSigV))
	if err != nil {
		return fmt.Errorf("couldn't open witnessed checkpoint with witness verifier(s): %v", err)
	}

	// Now form a PR with the cosigned CP.
	id := wcpID(*witnessNote)
	branchName := fmt.Sprintf("refs/heads/witness_%s", id)
	deleteBranch, err := gitCreateLocalBranch(forkRepo, headRef, branchName)
	if err != nil {
		return fmt.Errorf("failed to create git branch for PR: %v", err)
	}
	defer deleteBranch()

	wCp := &log.Checkpoint{}
	if _, err := wCp.Unmarshal([]byte(witnessNote.Text)); err != nil {
		return fmt.Errorf("failed to parse witnessed checkpoint: %v", err)
	}

	outputPath := filepath.Join(opts.distributorPath, "logs", opts.feederOpts.LogID, "incoming", fmt.Sprintf("checkpoint_%s", id))
	msg := fmt.Sprintf("Witness checkpoint@%v", wCp.Size)
	if err := gitCommitFile(workTree, outputPath, wRaw, msg, opts.gitUsername, opts.gitEmail); err != nil {
		return fmt.Errorf("failed to commit updated checkpoint.witnessed file: %v", err)
	}

	glog.V(1).Infof("git push -f origin/%s", branchName)
	if err := forkRepo.Push(&git.PushOptions{
		Force: true,
	}); err != nil {
		return fmt.Errorf("git push -f: %v", err)
	}

	glog.V(1).Info("Creating PR")
	return createPR(ctx, opts, ghCli, fmt.Sprintf("Witness @ %d", wCp.Size), opts.feederRepo.owner+":"+branchName, "master")
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

// repo represents an owner/repo fragment.
// Used in the options struct below.
type repo struct {
	owner    string
	repoName string
}

// String returns "<owner>/<repo>".
func (r repo) String() string {
	return fmt.Sprintf("%s/%s", r.owner, r.repoName)
}

// newRepo creates a new repo struct from an owner/repo fragment.
func newRepo(or string) (*repo, error) {
	s := strings.Split(or, "/")
	if l, want := len(s), 2; l != want {
		return nil, fmt.Errorf("can't split owner/repo %q, found %d parts want %d", or, l, want)
	}
	return &repo{
		owner:    s[0],
		repoName: s[1],
	}, nil
}

// options contains the various configuration and state required to perform a feedOnce call.
type options struct {
	gitUsername, gitEmail string
	githubAuthToken       string

	distributorRepo *repo
	feederRepo      *repo
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
	checkNotEmpty := func(v string, m string) {
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

	dr, err := newRepo(*distributorOwnerRepo)
	if err != nil {
		usageExit(fmt.Sprintf("--distributor_owner_repo invalid: %v", err))
	}
	fr, err := newRepo(*feederOwnerRepo)
	if err != nil {
		usageExit(fmt.Sprintf("--feeder_owner_repo invalid: %v", err))
	}

	fOpts, err := readFeederConfig(*feederConfigPath)
	if err != nil {
		glog.Exitf("Feeder config in %q is invalid: %v", *feederConfigPath, err)
	}

	return &options{
		distributorRepo: dr,
		feederRepo:      fr,
		distributorPath: *distributorRepoPath,
		githubAuthToken: githubAuthToken,
		gitUsername:     gitUsername,
		gitEmail:        gitEmail,
		witSigV:         fOpts.Witness.SigVerifier(),
		feederOpts:      *fOpts,
		feedInterval:    *interval,
	}
}

// setupFeederRepo clones a fork of the log repo, ready to be used to raise a PR against the log.
func setupFeederRepo(ctx context.Context, opts *options) (*git.Repository, error) {
	// Clone the fork of the log, so we can go on to make a PR against it.
	//   git clone -o origin "https://${GIT_USERNAME}:${FEEDER_GITHUB_TOKEN}@github.com/${fork_repo}.git" "${temp}"
	cloneOpts := &git.CloneOptions{
		URL: fmt.Sprintf("https://%v:%v@github.com/%s.git", opts.gitUsername, opts.githubAuthToken, opts.feederRepo),
	}

	var forkRepo *git.Repository
	var err error
	dest := opts.feederClonePath
	if opts.feederClonePath == "" {
		forkRepo, err = git.Clone(memory.NewStorage(), memfs.New(), cloneOpts)
		dest = "memory"
	} else {
		forkRepo, err = git.PlainClone(opts.feederClonePath, false, cloneOpts)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to clone fork repo %q: %v", cloneOpts.URL, err)
	}
	glog.V(1).Infof("Cloned %q into %q", cloneOpts.URL, dest)

	// Create a remote -> distributorOwnerRepo
	//  git remote add upstream "https://github.com/${distributor_repo}.git"
	distributorURL := fmt.Sprintf("https://github.com/%v.git", opts.distributorRepo)
	distributorRemote, err := forkRepo.CreateRemote(&config.RemoteConfig{
		Name: "upstream",
		URLs: []string{distributorURL},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to add remote %q to fork repo: %v", distributorURL, err)
	}
	glog.V(1).Infof("Added remote upstream->%q for %q:\n%v", distributorURL, opts.feederRepo, distributorRemote)

	if err = forkRepo.FetchContext(ctx, &git.FetchOptions{}); err != nil && err != git.NoErrAlreadyUpToDate {
		return nil, fmt.Errorf("failed to fetch repo %q: %v", *opts.feederRepo, err)
	}

	return forkRepo, nil
}

// authWithGithub returns a github client struct which uses the provided OAuth token.
// These tokens can be created using the github -> Settings -> Developers -> Personal Authentication Tokens page.
func authWithGithub(ctx context.Context, ghToken string) (*github.Client, error) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: ghToken})
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc), nil
}

// pullAndGetRepoHead ensures our local repo is up to date with the origin.
func pullAndGetRepoHead(r *git.Repository) (*plumbing.Reference, *git.Worktree, error) {
	wt, err := r.Worktree()
	if err != nil {
		return nil, nil, fmt.Errorf("workTree(%v) err: %v", r, err)
	}
	// Pull the latest commits from remote 'upstream'.
	if err := wt.Pull(&git.PullOptions{RemoteName: "upstream"}); err != nil && err != git.NoErrAlreadyUpToDate {
		return nil, nil, fmt.Errorf("git pull %v upstream err: %v", r, err)
	}
	// Get the master HEAD - we'll need this to branch from later on.
	headRef, err := r.Head()
	if err != nil {
		return nil, nil, fmt.Errorf("reading %v HEAD ref err: %v", r, err)
	}
	return headRef, wt, nil
}

// gitCreateLocalBranch creates a local branch on the given repo.
//
// Returns a function which will delete the local branch when called.
func gitCreateLocalBranch(repo *git.Repository, headRef *plumbing.Reference, branchName string) (func(), error) {
	// Create a git branch for the witnessed checkpoint to be added, named
	// using hex(checkpoint hash).  Construct a fully-specified
	// reference name for the new branch.
	branchRefName := plumbing.ReferenceName(branchName)
	branchHashRef := plumbing.NewHashReference(branchRefName, headRef.Hash())

	workTree, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get work tree: %v", err)
	}

	glog.V(1).Infof("git checkout -b %q", branchHashRef.Name())
	if err := workTree.Checkout(&git.CheckoutOptions{
		Branch: branchRefName,
		Create: true,
	}); err != nil {
		return nil, fmt.Errorf("git checkout %v: %v", branchRefName, err)
	}
	d := func() {
		if err := repo.Storer.RemoveReference(branchHashRef.Name()); err != nil {
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
	}

	return d, nil
}

// gitCommitFile creates a commit on a repo's worktree which overwrites the specified file path
// with the provided bytes.
func gitCommitFile(workTree *git.Worktree, path string, raw []byte, commitMsg string, username, email string) error {
	glog.Infof("Writing checkpoint (%d bytes) to %q", len(raw), path)
	if err := util.WriteFile(workTree.Filesystem, path, raw, 0644); err != nil {
		return fmt.Errorf("failed to write to %q: %v", path, err)
	}

	glog.V(1).Infof("git add %s", path)
	if _, err := workTree.Add(path); err != nil {
		return fmt.Errorf("failed to git add %q: %v", path, err)
	}

	glog.V(1).Info("git commit")
	_, err := workTree.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			// Name, Email required despite what we set up in git config earlier.
			Name:  username,
			Email: email,
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("git commit: %v", err)
	}
	return nil
}

// createPR creates a pull request. Based on: https://godoc.org/github.com/google/go-github/github#example-PullRequestsService-Create
func createPR(ctx context.Context, opts *options, ghCli *github.Client, title, commitBranch, prBranch string) error {
	if title == "" {
		return errors.New("missing `title`, won't create PR")
	}

	newPR := &github.NewPullRequest{
		Title:               github.String(title),
		Head:                github.String(commitBranch),
		Base:                github.String(prBranch),
		MaintainerCanModify: github.Bool(true),
	}

	prJSON, err := json.Marshal(newPR)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON for new PR request: %v", err)
	}
	glog.V(1).Infof("Creating PR:\n%s", prJSON)

	pr, _, err := ghCli.PullRequests.Create(ctx, opts.distributorRepo.owner, opts.distributorRepo.repoName, newPR)
	if err != nil {
		return err
	}

	glog.Infof("PR created: %s", pr.GetHTMLURL())
	return nil
}

// feederConfig is the format of the feeder config file.
type feederConfig struct {
	// The LogID used by the witnesses to identify this log.
	LogID string `json:"log_id"`
	// PublicKey associated with LogID.
	LogPublicKey string `json:"log_public_key"`
	// LogURL is the URL of the root of the log.
	LogURL string `json:"log_url"`
	// Witness defines the target witness
	Witness struct {
		URL       string `json:"url"`
		PublicKey string `json:"public_key"`
	} `json:"witness"`
}

// readFeederConfig parses the named file into a FeedOpts structure.
func readFeederConfig(f string) (*impl.FeedOpts, error) {
	c, err := ioutil.ReadFile(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}
	cfg := &feederConfig{}
	if err := json.Unmarshal(c, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %v", err)
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
