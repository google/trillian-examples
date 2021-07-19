# witness_pr

`witness_pr` is a GitHub Action for handling incoming PRs from _witnesses_ containing a
co-signed log checkpoint which the witness has verified is consistent with earlier
log checkpoints it has seen.

This action would be used by a serverless log which was participating in an ecosystem
with non-addressable witnesses.

## Overview

The serverless log creates a `checkpoint` file as part of the integration step. This file
is signed by the log and commits to the contents of the log at the point at which it was
created.

Witnesses are entities which work to help prevent the possibility of a log undertaking an
undetected split-view attack, they do this by verifying consistency of the log checkpoints
they see, and cosigning those they find to be consistent.
Entities which rely on the contents of the log can thereby trust that the view of the log
_they_ see has also been seen by _at least_ the set of witnesses which have cosigned the
checkpoint they hold.

In order to facilitate the distribution of cosigned checkpoints, witnesses can return their
cosigned checkpoints to the log, which in turn can serve them alongside the original `checkpoint`.
Since the checkpoint signature format supports multiple signatures, the log can coalesce
signatures on a given checkpoint from multiple witnesses into a single file: `checkpoint.witnessed`.

Note that at any given time, `checkpoint.witnessed` may represent a checkpoint which is
equivalent to `checkpoint`, or may represent an _earlier_ checkpoint (e.g. if the log has
just issued a new `checkpoint` and there have been no witness submissions for it as yet).

For more details on witnessing strategies as well as witness implementation(s), see the
[witness](github.com/google/trillian-examples/witness) package.

## Operation

The `witness_pr` action requires a copy of the PR branch, as well as a pristine checkout of
the log's `master` (or `main`) branch.
Public keys for known witnesses should already be committed on the `master` branch.

The `witness_pr` action:
1. checks that there is only one modified file present in the PR: `checkpoint.witnessed`
2. attempts to combine signatures from the PR's `checkpoint.witnessed` file with an existing
   `checkpoint.witnessed` file in the `master` branch, and updates the file in the PR with
   the union of the signatures.
3. if step (2) failed (e.g. because the log published an updated `checkpoint` file which the
   witness has signed), then it attempts to combine signatures from the PR's `checkpoint.witnessed`
   file with the log signature on the `checkpoint` file in the log's `master` branch, and updates
   the file in the PR with the union of the signatures.

The combining of signatures is done using the
[`combine_signatures`](https://github.com/google/trillian-examples/serverless/cmd/combine_signatures) tool.

## Usage

### Inputs

Input          | Description
---------------|-----------------
`log_dir`      | Path to the root of the serverless log files in this repo.
`pr_repo_root` | Location within `${GITHUB_WORKSPACE}` where the PR branch is checked out.
`pristine_repo_root` | Location within `${GITHUB_WORKSPACE}` where the pristine copy of the master repo is checked out.
`witness_key_files` | Path glob matching the set of known witness keys in note format.
`log_public_key` | The serverless log's public key (note: the key itself, not a path to a file containing the key).

To use this PR with your log, create a `.github/workflows/witness_pr.yaml` file with the
following contents:

```yaml
on:
  pull_request:
    branches:
      - master

jobs:
  leaf_validator_job:
    runs-on: ubuntu-latest
    name: Handle witness PR
    steps:
    # Checkout the PR branch into pr/
    - uses: actions/checkout@v2
      with:
        path: 'pr'
        fetch-depth: 0
    # Checkout the log's master branch into pristine/
    - uses: actions/checkout@v2
      with:
        path: 'pristine'
        ref: 'master'
        fetch-depth: 0
    # Attempt to combine the signatures on the checkpoint in the PR with the log's latest checkpoint/checkpoint.witnessed file
    - name: Validate and combine
      id: validate_combine
      uses: google/trillian-examples/serverless/deploy/github/witness_pr@master
      with:
        log_dir: './log'
        pr_repo_root: 'pr'
        pristine_repo_root: 'pristine'
        witness_key_files: 'witnesskeys/*pub'
        log_public_key: ${{ secrets.SERVERLESS_LOG_PUBLIC_KEY }}
    # Update the PR with merge results if necessary
    - uses: stefanzweifel/git-auto-commit-action@v4
      with:
        repository: 'pr'
        commit_user_name: Serverless Bot
        commit_user_email: actions@github.com
        commit_author: Serverless Bot <actions@github.com>
        commit_message: Witness checkpoint merge
```
