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

// feeder/github is a ....
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/golang/glog"
	//  "github.com/google/go-github"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	//  "github.com/go-git/go-git/v5/plumbing"
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
		glog.Exitf("Error during set-up: %v", err)
	}

	//   3. auth to Github
	//
	//
	// Main feeder loop.
	//   1. run feeder to check if there are new signatures
	//   2. if not, sleep until next time to check.
	//   3. if so, move output from feeder onto new branch

	//   4. make commit to branch
	//   5. create checkpoint PR
	//   6. clean up
	//   7. --end of cycle--
	fmt.Println("[Starting feeding]---------------------------------------------------")
	for {
		if err = createCheckpointPR(ctx, opts, witnessRepo); err != nil || opts.feederInterval == 0 {
			break
		}
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
		return nil, fmt.Errorf("Failed to clone fork repo %q: %v", forkLogURL, err)
	}
	glog.Infof("Cloned %q into %q", forkLogURL, opts.witnessClonePath)

	// Create a remote -> logOwnerRepo
	//  git remote add upstream "https://github.com/${log_repo}.git"
	logURL := fmt.Sprintf("https://github.com/%v.git", opts.logOwnerRepo)
	logRemote, err := forkRepo.CreateRemote(&config.RemoteConfig{
		Name: "upstream",
		URLs: []string{logURL},
	})
	if err != nil {
		return nil, fmt.Errorf("Failed to add remote %q to fork repo: %v", logURL, err)
	}
	glog.Infof("Added remote upstream->%q for %q: %v", logURL, opts.witnessOwnerRepo, logRemote)

	// Ensure the forkRepo config has git username and email set.
	//  git config user.name "${GIT_USERNAME}"
	//  git config user.email "${GIT_EMAIL}"
	cfg, err := forkRepo.Config()
	if err != nil {
		return nil, fmt.Errorf("Failed to read config for repo %q: %v", opts.witnessOwnerRepo, err)
	}
	cfg.User.Name = opts.gitUsername
	cfg.User.Email = opts.gitEmail
	if err = forkRepo.SetConfig(cfg); err != nil {
		return nil, fmt.Errorf("Failed to update config for repo %q: %v", opts.witnessOwnerRepo, err)
	}

	//  git fetch --all
	if err = forkRepo.FetchContext(ctx, &git.FetchOptions{}); err != nil && err != git.NoErrAlreadyUpToDate {
		return nil, fmt.Errorf("Failed to fetch repo %q: %v", *witnessOwnerRepo, err)
	}

	//  git branch -u upstream/master
	// TODO: hmm: not sure if the go-git library lets me do this.  At this point the
	// forkRepo's local .git/config has:
	/*
	   [core]
	   	bare = false
	   [remote "origin"]
	   	url = https://phad:<personal-api-token>@github.com/phad/serverless-test.git
	   	fetch = +refs/heads/*:refs/remotes/origin/*
	   [remote "upstream"]
	   	url = https://github.com/AlCutter/serverless-test.git
	   	fetch = +refs/heads/*:refs/remotes/upstream/*
	   [branch "master"]
	   	remote = origin
	   	merge = refs/heads/master
	   [user]
	   	name = phad
	   	email = hadfieldp@google.com
	*/
	// so am I expecting the [branch "master"] section to look like:
	/*
	   [branch "master"]
	   	remote = upstream
	   	merge = refs/heads/master
	*/
	return forkRepo, nil
}

func createCheckpointPR(ctx context.Context, opts *options, forkRepo *git.Repository) error {

	// 1. git pull

	// 2. kick off feeder. (TODO: refactor feeder to a library so we don't need to spawn a process)
	witnessed, err := spawnFeeder(ctx, opts)
	if err != nil {
		return err
	}

	// 3. did feeder add a signuture or update the existing checkpoint.witnessed file?
	if !witnessed {
		fmt.Println("No signatures added")
		return nil
	}

	// TODO...

	fmt.Println("[Feed cycle complete]------------------------------------------------")
	return nil
}

func spawnFeeder(ctx context.Context, opts *options) (bool, error) {
	// TODO: kick off feeder process
	// -config_file="${feeder_conf}" --logtostderr --input="https://raw.githubusercontent.com/${log_repo}/master/${log_path}/checkpoint" --output="${temp}/${log_path}/checkpoint.witnessed" -v 2
	cmd := exec.Command("feeder", fmt.Sprintf("--config_file=%v", opts.feederConfigFile),
		fmt.Sprintf("--input=https://raw.githubusercontent.com/%v/master/%v/checkpoint", opts.logOwnerRepo, opts.logRepoPath),
		fmt.Sprintf("--output=%v/checkpoint.witnessed", path.Join(opts.witnessClonePath, opts.logRepoPath)),
		"-v=2", "-logtostderr")

	glog.Infof("cmd: \n%v", cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		glog.Errorf("feeder output:\n%v", string(out))
		return false, err
	}

	fmt.Sprintf("feeder output:\n%v\n", out)
	return true, nil
}
