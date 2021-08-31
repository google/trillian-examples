# Deploying serverless checkpoint distributor on GitHub

> :warning: This is very experimental.

Similar to serverless logs, we can deploy a file-based witnessed checkpoint
distributor on GitHub too.

For more details on witnessing, and the various roles involved (including
`distributor`), please see the [witness README](/witness) in this repo.

This directory provides some GitHub Actions and documentation to help with that.

For the moment, we'll assume that you have a fresh GitHub repo which you'll
dedicate to the sole use of being a distributor, although it should also work fine
in a repo containing other files too.

## GitHub Actions

### PR validation

The following GitHub Actions workflow can be used to automatically handle
incoming witness PRs:

Create a `.github/workflows/serverless_pr.yaml` file with the following config:

```yaml
name: Serverless PR
on:
  pull_request_target:
    branches:
      - master

env:
  # Update this to the location of your distributor root directory if different:
  DISTRIBUTOR_ROOT: "distributor"

jobs:
  changes:
    runs-on: ubuntu-latest
    name: Serverless PR handler
    outputs:
      # Add extra outputs to correspond to any additions to the matched patterns in the filter step below.
      distributor: ${{ steps.filter.outputs.distributor }}
    steps:
      - name: Match files in PR against rules
        id: filter
        uses: dorny/paths-filter@v2
        with:
          list-files: shell
          # Can add more patterns here if necessary, don't forget to update the outputs above if you do so!
          filters: |
            distributor:
              - '${{ env.DISTRIBUTOR_ROOT }}/logs/*/incoming/*'

# This job does a more detailed check on the contents of any incoming checkpoints added.
# Run this job only when we've detected a witnessed checkpoint
  distributor_pr_validator:
    needs: changes
    if: ${{ needs.changes.outputs.distributor == 'true' }}
    runs-on: ubuntu-latest
    name: Handle witness PR
    steps:
      - uses: actions-ecosystem/action-add-labels@v1
        with:
          labels: Witness
      - uses: actions/checkout@v2
        with:
          fetch-depth: 0
          ref: "refs/pull/${{ github.event.number }}/merge"
      - name: Validate distributor PR
        uses: google/trillian-examples/serverless/deploy/github/distributor/combine_witness_signatures@master
        with:
          distributor_dir: '${{ env.DISTRIBUTOR_ROOT }}'
          config: '${{ env.DISTRIBUTOR_ROOT }}/config.json'
          dry_run: true
```

### Updating distributor state

Once "incoming checkpoint" PRs are merged, we need to integrate these checkpoints into the stored
distributor state.

Add the following config to the `.github/workflows/serverless_merge_master.yaml` file:

```yaml
on:
  push:
    branches:
      - master

env:
  # Update this to the location of your distributor root directory if different:
  DISTRIBUTOR_ROOT: "distributor"

jobs:
  combine_witness_sigs:
    runs-on: ubuntu-latest
    name: Combine witness signatures
    steps:
    - uses: actions/checkout@v2
    # Attempt to combine witness signatures with the log checkpoint.
    - name: Combine witness signatures
      id: combine_witness_signatures
      uses: google/trillian-examples/serverless/deploy/github/distributor/combine_witness_signatures@master
      with:
          distributor_dir: '${{ env.DISTRIBUTOR_ROOT }}'
          config: '${{ env.DISTRIBUTOR_ROOT }}/config.json'
    - uses: stefanzweifel/git-auto-commit-action@v4
      with:
        commit_user_name: Serverless Bot
        commit_user_email: actions@github.com
        commit_author: Serverless Bot <actions@github.com>
        commit_message: Automatically merge witness signatures
```

## Try it out yourself

To try it out:

1. Create a fresh github repo to contain a distributor, and clone locally.
2. Initialise the distributor state:
    1. we'll use a directory called `distributor` in our repo to
       store the state files
    2. create a `config.json` file ([example](combine_witness_signatures/examples_config.json))
    3. add and commit the config file to your new repo:

       ```bash
       git add --all
       git commit -m "Initialise my distributor"
       ```

3. Place the above github action configs into the `.github/workflows` directory in
   your distributor repo, and commit that too.
4. Push these commits up to github.

Now you can raise "incoming checkpoint" PRs which drop cosigned checkpoints into the
`distributor/logs/<log_id>/incoming` directory, whereupon the `ditributor_pr_validator`
action should check the contents.
Once the "incoming checkpoint" PRs are merged you should see the
`combine_witness_signatures` action running in response (check the `Actions` tab on
your github repo's page).

## Going further

We could take it further, and have the `serverless_pr` action
automatically merge valid PRs and close others, but this is currently left as
an exercise for the reader.
