# Deploying serverless logs on GitHub

> :warning: This is experimental.

Since serverless logs are just files, we can easily use GitHub to store and
make these available on-line - just check the files in and use GitHub's
`https://raw.githubusercontent.com/...` URLs to serve them.

To update these logs you'd clone the repository containing the log, use the
`sequence` and `integrate` tools to grow the log, and then create a PR with
the deltas.

But can we go further?

Yes, dear reader; read on!

## GitHub Actions

We can configure our log repository to use GitHub Actions to automate much of
this process.

### Sequencing & integration

Here is a GitHub actions workflow config which will automate the sequencing
and integration of "leaves" which have been added to the `leaves/pending`
directory of a serverless log.

> :shipit: Note that it expects a pair of GitHub secrets called
`SERVERLESS_LOG_PRIVATE_KEY` and `SERVERLESS_LOG_PUBLIC_KEY` to exist, see 
the [GitHub secrets docs](https://docs.github.com/en/actions/reference/encrypted-secrets#creating-encrypted-secrets-for-a-repository)
for details on how to do this.

`integrate_leaves.yaml`

```yaml
on:
  push:
    branches:
      - master

jobs:
  sequence_and_integrate_job:
    runs-on: ubuntu-latest
    name: Sequence and integrate pending log entries
    steps:
    - uses: actions/checkout@v2
    - name: Sequence and integrate step
      id: sequence_and_integrate
      uses: google/trillian-examples/serverless/deploy/github/sequence_and_integrate@master
      with:
        log_dir: './log'
        ecosystem: 'Log Checkpoint v0'
      env:
        SERVERLESS_LOG_PRIVATE_KEY: ${{ secrets.SERVERLESS_LOG_PRIVATE_KEY }}
        SERVERLESS_LOG_PUBLIC_KEY: ${{ secrets.SERVERLESS_LOG_PUBLIC_KEY }}
    - uses: stefanzweifel/git-auto-commit-action@v4
      with:
        commit_user_name: Serverless Bot
        commit_user_email: actions@github.com
        commit_author: Serverless Bot <actions@github.com>
        commit_message: Automatically sequence and integrate log
```

This action will be invoked every time a push is made to master, and undertakes
the following steps:

- check out the repo containing the log,
- sequences all files it finds in `log/leaves/pending` and then deletes the files,
- integrates all sequenced but unintegrated leaves
- commits all changes from the sequencing/integration,
- pushes this commit to master, thereby updating the public state of the log repo.

Specifying `ecosystem` input parameter is optional. If not specified the default
`Log Checkpoint v0` string will be used.

### Verifying "queue leaf" PRs

Here is a GitHub actions workflow config which will automate the validation of
incoming "queue leaf" request PRs, it uses the `leaf_validator` action which
does the bare minimum to demonstrate the idea - if you were doing this for real
you'd likely want to validate format, signatures, etc. too.

`leaves_pr.yaml`

```yaml
on: 
  pull_request:
    branches:
      - master

jobs:
  leaf_validator_job:
    runs-on: ubuntu-latest
    name: Validate pending leaves
    steps:
    - uses: actions/checkout@v2
      with:
         fetch-depth: 0
    - name: Leaf validator step
      id: leaf_validator
      uses: google/trillian-examples/serverless/deploy/github/leaf_validator@master
      with:
        log_dir: './log'
```

## Try it out yourself

To try it out:

1. Create a fresh github repo to contain a log, and clone locally.
2. Create your own log key pair, using the `generate_keys` tool, add the generated keys
   to your Github repo's secrets as `SERVERLESS_LOG_PUBLIC_KEY` and
   `SERVERLESS_LOG_PRIVATE_KEY`.
2. Initialise the log state:
    1. we'll use a directory called `log` in our repo to
       store the state files
    2. run the `integrate` tool with the `--initialise` flag:
      `go run ./serverless/cmd/integrate --initialise --storage_dir=<path/to/your/repo>/log --logtostderr`
    3. now commit the files it created to your new repo:

       ```bash
       git add --all
       git commit -m "Initialise my log"
       ```

3. Place the above github action configs into the `.github/workflows` directory in
   your log repo, and commit that too.
4. Push these commits up to github.

Now you can raise "pending leaf" PRs which drop files into the
`log/leaves/pending` directory, whereupon the `Validate pending leaves` action
should check the contents, and when the "pending leaf" PRs are merged you
should see the `Sequence and integrate` action running in response (check the
`Actions` tab on your github repo's page).

You can use the `client` tool to interact with your new log by using the GitHub
raw URL address of your log's repo with the `storage_url` parameter:

```bash
$ go run ./serverless/cmd/client/ --logtostderr --log_url=https://raw.githubusercontent.com/AlCutter/serverless-test/master/log/ -v=2 --cache_dir="" inclusion ./CONTRIBUTING.md
I0430 17:49:33.924422 3389781 client.go:117] Local log state cache disabled
I0430 17:49:34.368392 3389781 client.go:156] Leaf "./CONTRIBUTING.md" found at index 1
I0430 17:49:34.648373 3389781 client.go:172] Built inclusion proof: [0xfe4ac37cf74158146b2ab74af030687428fdc59637c5e19a66cdd3a36b29d3e1 0x5dafd147891541a65988be686b77a9cf41f8760b5d10b99f09dddba53c995670]
I0430 17:49:34.648439 3389781 client.go:178] Inclusion verified in tree size 3, with root 0x676386dbcaec44d69736e1bf709d6c1e5492874e78bbf4920b79944bcfb08927
```

## Going further

We could take it further, and have the `validate pending leaves` action
automatically merge valid PRs and close others, but this is currently left as
an exercise for the reader.
