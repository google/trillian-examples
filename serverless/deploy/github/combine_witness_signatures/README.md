# combine_witness_signatures

`combine_witness_signatures` is a GitHub Action for combining signatures on checkpoints
cosigned by known witnesses.

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

PRs containing cosigned checkpoint files under the log's `.../witness` directory are
raised by witnesses, and these PRs are validated by the [witness_pr](../witness_pr) action,
and merged.

The `combine_witness_signatures` action:
1. is triggered on pushes to `master`
2. attempts to merge witness cosigned checkpoints under `.../witness` with
   either `checkpoint` or `checkpoint.witnessed`:
  1. if there are sufficient witness signatures to promote `checkpoint` to
      `checkpoint.witnessed`, then merge and do so
  2. otherwise, attempt to merge witness signaures with `checkpoint.witnessed` if
     it exists.

## Usage

### Inputs

Input                         | Description
------------------------------|-----------------
`log_dir`                     | Path to the root of the serverless log files in this repo.
`witness_key_files`           | Path glob matching the set of known witness keys in note format.
`log_public_key`              | The serverless log's public key (note: the key itself, not a path to a file containing the key).
`required_witness_signatures` | Minimum number of witness signatures required to promote `checkpoint` to `checkpoint.witnessed`
`delete_consumed`             | Whether to delete the cosigned checkpoints under `witness/` once they're consumed.

To use this PR with your log, create a `.github/workflows/combine_witness_sigs.yaml` file with the
following contents (replace `<PUT LOG PUBLIC KEY HERE>` with the literal log public key - don't
try to populate this from a GitHub secret as it won't be visible to on PRs from witness forks!):

```yaml
on:
  push:
    branches:
      - master

jobs:
  combine_witness_sigs:
    runs-on: ubuntu-latest
    name: Combine witness signatures
    steps:
    - uses: actions/checkout@v2
    # Attempt to combine witness signatures with the log checkpoint.
    - name: Combine witness signatures
      id: combine_witness_sigs
      uses: google/trillian-examples/serverless/deploy/github/combine_witness_signatures@master
      with:
          log_dir: './log'
          # NOTE: this should point to the directory containing the public keys of known witnesses
          witness_key_files: 'witnesskeys/*pub'
          # NOTE: Replace this with the literal log public key
          log_public_key: '<PUT LOG PUBLIC KEY HERE>'
          required_witness_signatures: 1
    - uses: stefanzweifel/git-auto-commit-action@v4
      with:
        commit_user_name: Serverless Bot
        commit_user_email: actions@github.com
        commit_author: Serverless Bot <actions@github.com>
        commit_message: Automatically merge witness signatures
```
