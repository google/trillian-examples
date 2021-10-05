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
  pull_request:
    branches:
      # This is the name of the primary branch, which may be `main` for newer repos.
      - master

env:
  # Update this to the location of your distributor root directory if different:
  DISTRIBUTOR_ROOT: "distributor"

jobs:
  changes:
    runs-on: ubuntu-latest
    name: Serverless PR handler
    outputs:
      # Add extra outputs here to match any additions to the matched patterns in the filter step below.
      distributor_incoming: ${{ steps.filter.outputs.distributor_incoming }}
    steps:
      - name: Check for changed files
        id: filter
        uses: dorny/paths-filter@v2
        with:
          list-files: shell
          filters: |
            distributor_incoming:
              - added: '${{ env.DISTRIBUTOR_ROOT }}/logs/*/incoming/*'
            distributor_private:
              - '${{ env.DISTRIBUTOR_ROOT }}!(/logs/*/incoming/*)'

      - name: Detect distributor structure changes
        if: steps.filter.outputs.distributor_private == 'true'
        run: |
          for i in ${{ steps.filter.outputs.distributor_private_files }}; do
            echo "::error file=${i}::Modified protected distributor structure"
          done
          exit 1

# Run this job only when we've detected a distributor checkpoint PR
  distributor_validator:
    needs: changes
    if: ${{ needs.changes.outputs.distributor_incoming == 'true' }}
    runs-on: ubuntu-latest
    name: Handle distributor PR
    steps:
      - uses: actions/checkout@v2
        with:
          fetch-depth: 0
          ref: "refs/pull/${{ github.event.number }}/merge"
      - name: Combine witness signatures (dry run)
        id: combine_witness_signatures_dry_run
        uses: google/trillian-examples/serverless/deploy/github/distributor/combine_witness_signatures@HEAD
        with:
            distributor_dir: './distributor'
            config: './distributor/config.yaml'
            dry_run: true
      # OPTIONAL: Store PR number (only needed if we're using the automerge action below)
      - id: save_metadata
        name: Save PR number
        run: |
          D=$(mktemp -d)/pr_metadata
          mkdir -p ${D}
          echo ${{ github.event.number }} > ${D}/NR
          echo "::set-output name=metadata_dir::${D}"
      - uses: actions/upload-artifact@v2
        with:
          name: pr_metadata
          path: ${{ steps.save_metadata.outputs.metadata_dir }}
      # End OPTIONAL.
                                                                    
```

### Updating distributor state

Once "incoming checkpoint" PRs are merged, we need to integrate these checkpoints into the stored
distributor state.

Add the following config to the `.github/workflows/serverless_merge_master.yaml` file:

```yaml
name: Integrate Incoming Checkpoints
on:
  push:
    branches:
      # This is the name of the primary branch, which may be `main` for newer repos.
      - master
  workflow_dispatch:
  # Enable this if you set up automerge as below
  #workflow_run:
  #  workflows: ["Automerge PR"]
  #  types:
  #    - completed

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
          config: '${{ env.DISTRIBUTOR_ROOT }}/config.yaml'
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
    2. create a `config.yaml` file ([example](combine_witness_signatures/example_config.yaml))
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

## Going further: automated PR merges

Automating the merging incoming witness PRs can help to take some friction
out of the process and help to get cosigned checkpoints out to clients quicker
than with manual review.

An example config to do this is given below:

`.github/workflows/automerge.yaml`
```yaml
name: Automerge PR
on:
  # Trigger this action when other actions have completed.
  # workflow_run gives us the elevated privileges necessary to merge PRs, etc.
  workflow_run:
    # Note that this should match the name of the workfle triggered by distributor PRs being raised:
    workflows: ["Serverless PR"]
    types:
      - completed

env:
  # Update this to the location of your distributor root directory if different:
  DISTRIBUTOR_ROOT: "distributor"

jobs:
  on-success:
    runs-on: ubuntu-latest
    # Only run when the trigger event was the successful completion of the "Serverless PR" workflow.
    if: >
      ${{ github.event.workflow_run.event == 'pull_request' &&
          github.event.workflow_run.conclusion == 'success' }}
    steps:
      # Fetch PR number stored by the optional step in the Serverless PR workflow above.
      - name: 'Fetch PR metadata artifact'
        uses: actions/github-script@v3.1.0
        with:
          script: |
            var artifacts = await github.actions.listWorkflowRunArtifacts({
               owner: context.repo.owner,
               repo: context.repo.repo,
               run_id: ${{github.event.workflow_run.id }},
            });
            var matchArtifact = artifacts.data.artifacts.filter((artifact) => {
              return artifact.name == "pr_metadata"
            })[0];
            var download = await github.actions.downloadArtifact({
               owner: context.repo.owner,
               repo: context.repo.repo,
               artifact_id: matchArtifact.id,
               archive_format: 'zip',
            });
            var fs = require('fs');
            fs.writeFileSync('${{github.workspace}}/pr_metadata.zip', Buffer.from(download.data));
      - name: 'Grab PR number'
        id: pr_metadata
        run: |
          unzip pr_metadata.zip
          echo "::set-output name=pr::$(cat NR)"

      - uses: actions-ecosystem/action-add-labels@v1
        with:
          labels: Automerge
          number: ${{ steps.pr_metadata.outputs.pr }}

      - name: automerge
        uses: "pascalgn/automerge-action@v0.14.3"
        env:
          GITHUB_TOKEN: "${{ secrets.GITHUB_TOKEN }}"
          MERGE_LABELS: Automerge
          MERGE_METHOD: rebase
          MERGE_DELETE_BRANCH: true
          PULL_REQUEST: ${{ steps.pr_metadata.outputs.pr }}
```

