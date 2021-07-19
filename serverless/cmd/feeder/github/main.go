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
  "flag"
  "fmt"
  "os"
  "time"

	"github.com/golang/glog"
//  "github.com/google/go-github"
  "github.com/go-git/go-git/v5"
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

  // Make a tempdir to clone the witness (forked) log into.
  forkedLogDir, err := os.MkdirTemp(os.TempDir(), "feeder-github")
  if err != nil {
    glog.Exitf("Error creating temp dir: %v", err)
  }

  // Preparation:
  //   2. clone the fork of the log, so we can go on to make a PR against it
  logURL := fmt.Sprintf("https://github.com/%v", *logOwnerRepo)
  glog.Infof("Cloning %q... into %q", logURL, forkedLogDir)
  _, err = git.PlainClone(forkedLogDir, false, &git.CloneOptions{
		URL: logURL,
	})
  if err != nil {
    glog.Exitf("Failed to clone repo %q: %v", logURL, err)
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

    CreateCheckpointPR()
}


func CreateCheckpointPR() {
  fmt.Println("creating checkpoint PR ...")

  // 1.

  //
}
