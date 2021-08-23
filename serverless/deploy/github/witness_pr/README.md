# witness_pr

`witness_pr` is a GitHub Action for validating incoming PRs from _witnesses_ containing a
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

Public keys for known witnesses should already be committed on the `master` branch.

The `witness_pr` action:
1. Ensures that if the PR contains any witnessed checkpoints, then it must contain ONLY witnessed checkpoints.
2. Checks that the new witnessed checkpoints are signed by the log
3. Checks that the new witnessed checkpoints are additionally signed by at least one known witness.

TODO(al): This action could also check whether the checkpoint is equivalent to `checkpoint` or `checkpoint.witnessed`.

Note that the witnessed checkpoint is not merged with `checkpoint.witnessed` at this stage,
instead, that step once the witness PR has been merged, and is handled by the
[combine_witness_signatures](../combine_witness_signatures) action.
## Usage

### Inputs

Input          | Description
---------------|-----------------
`log_dir`      | Path to the root of the serverless log files in this repo.
`witness_key_files` | Path glob matching the set of known witness keys in note format.
`log_public_key` | The serverless log's public key (note: the key itself, not a path to a file containing the key).

To use this PR with your log, create a `.github/workflows/witness_pr.yaml` file with the
following contents (replace `<PUT LOG PUBLIC KEY HERE>` with the literal log public key - don't
try to populate this from a GitHub secret as it won't be visible to on PRs from witness forks!):

```yaml
on:
  pull_request:
    branches:
      - master

jobs:
  witness_pr_validator:
    runs-on: ubuntu-latest
    name: Handle witness PR
    steps:
    # Checkout the PR branch
    - uses: actions/checkout@v2
      with:
        fetch-depth: 0
    # Verify the witnessed checkpoints are in the correct location and have the right signatures
    - name: Validate witness PR
      id: validate_witness_pr
      uses: google/trillian-examples/serverless/deploy/github/witness_pr@master
      with:
        log_dir: './log'
        # NOTE: this should point to the directory containing the public keys of known witnesses
        witness_key_files: 'witnesskeys/*pub'
        # NOTE: Replace this with the literal log public key (don't use a GitHub secrets variable here)
        log_public_key: '<PUT LOG PUBLIC KEY HERE>'
```
