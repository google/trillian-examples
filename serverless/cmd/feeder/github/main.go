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
  "fmt"
)

// TODO: copied from feed-to-github; adapt as necessary.
const usage = `Usage:
 feed-to-github <log_github_owner/repo> <log_path> <feeder_config_file> [interval_seconds]

Where:
 <log_github_owner/repo> is the repo owner/fragment from the repo URL.
     e.g. github.com/AlCutter/serverless-test -> AlCutter/serverless-test
 <witness_github_owner/repo> is the repo owner/fragment of the log fork to use for the PR branch.
 <log_repo_path> is the path from the root of the rep where the log files can be found,
 <feeder_config_file> is the path to the config file for the serverless/cmd/feeder command.
 [interval_seconds] if set, the script will continously feed and (if needed) create witness PRs sleeping
     the specified number of seconds between attempts. If not provided, the tool does a one-shot feed.

EOF
`

func main() {
    fmt.Println("Github feeder.")

    // Preparation:
    //   1. Validate args
    //   2. clone the fork of the log, so we can go on to make a PR against it
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
