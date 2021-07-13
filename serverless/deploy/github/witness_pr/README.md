# witness_pr

`witness_pr` is a GitHub Action for handling incoming PRs from _witnesses_ containing a
co-signed log checkpoint which the witness has verified is consistent with earlier
log checkpoints it has seen.

This action would be used by a serverless log which was participating in an ecosystem
with non-addressable witnesses.

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
`pr_repo_root` | Location within `${GITHUB_WORSPACE}` where the PR branch is checked out.
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
      uses: AlCutter/trillian-examples/serverless/deploy/github/witness_pr@serverless_recipe_witness_pr
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