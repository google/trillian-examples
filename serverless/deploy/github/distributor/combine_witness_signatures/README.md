# combine_witness_signatures

`combine_witness_signatures` is a GitHub Action for combining signatures on checkpoints
cosigned by known witnesses.

This action would be used by a serverless witness distributor.

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

A _distributor_ makes these cosigned checkpoints available to log clients.

This GitHub Action implements a serverless distributor.
For more details on witnessing strategies as well as witness implementation(s), see the
[witness](github.com/google/trillian-examples/witness) package.

## Operation

A JSON formatted config file must be created and checked-in to the repo.
An example config file is shown below:

```json
{
        "MaxWitnessSignatures": 5,

        "Witnesses": [
                "can-I-get-a-witness+b97a1e56+AZkpOlwZwR+wwasAENZwIa98ufmWmzlq0Tx0XN7voU6X",
                "witness-over-here+29c4e8f4+AUbwUCBUM2sDdHeiKUrp6LnMErE7GEz0iH+0WbgbJZxx",
                "wolsey-bank-alfred+0336ecb0+AVcofP6JyFkxhQ+/FK7omBtGLVS22tGC6fH+zvK5WrIx"
        ],

        "Logs": [
                {
                        "ID": "test",
                        "PublicKey": "github.com/AlCutter/serverless-test/log+28035191+AVtQ/9lW+g90rQY3+pODJvMQ8X/tTvh/EuvCDLSmUk4S"
                }
        ]
}
```

PRs containing cosigned checkpoint files under the distributor's `.../logs/<logID>/incoming` directory are
raised by witnesses, validated, and merged.

Once these PRs are merged, this action:
1. is triggered on pushes to `master`
2. attempts to combine the checkpoints present for a given log with the ones
   from the incoming directory
3. produces one or more files containing checkpoints with merged signatures.

The output files are named `checkpoint.0`, `checkpoint.1`, etc. and contain the largest
checkpoint seen which has _at least_ the number of witness cosignatures specified by the
file name. `checkpoint.0` will always have the largest checkpoint seen, regardless of whether
or not it's been cosigned by witnesses.

## Usage

### Inputs

Input             | Description
------------------|-----------------
`distributor_dir` | Path to the root of the distributor directory in this repo.
`config`          | Path of distributor config file.
`dry_run`         | Will not modify on-disk state if set to true.

To use this PR with your log, create a `.github/workflows/distributor_master.yaml` file with the
following contents:

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
    - name: Combine witness signatures
      id: combine_witness_signatures
      uses: AlCutter/trillian-examples/serverless/deploy/github/distributor/combine_witness_signatures@serverless_distributor
      with:
          distributor_dir: './distributor'
          config: './distributor/config.json'
    - uses: stefanzweifel/git-auto-commit-action@v4
      with:
        commit_user_name: Serverless Bot
        commit_user_email: actions@github.com
        commit_author: Serverless Bot <actions@github.com>
        commit_message: Automatically merge witness signatures
```