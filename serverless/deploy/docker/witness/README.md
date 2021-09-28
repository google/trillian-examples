Witness
-------

This [docker-compose.yaml](docker-compose.yaml) file can be used to spin up a witness daemon.

## GitHub access

You'll need to create a GitHub
[Personal Access Token](https://docs.github.com/en/github/authenticating-to-github/keeping-your-account-and-data-secure/creating-a-personal-access-token)
for the feeder to use to create a PR on the log repo with the witnessed checkpoint.

**Important:**
When you create this token, tick **only** the `public_repo` checkbox - this witness does not need
any extra priviledges beyond this basic level of authorization.

## Configuration and running

You should set the following environmemt variables (either `export` or with a `.env` file):

Variable Name                   | Required | Description
--------------------------------|:--------:|-------------------------------------------------
`SERVERLESS_DISTRIBUTOR_REPO`   | yes      | The GitHub "<owner/repo>" fragment of the serverless distributor
`SERVERLESS_DISTRIBUTOR_FORK`   | yes      | The GitHub "<owner/repo>" fragment of the witness' fork of the serverless distributor
`SERVERLESS_DISTRIBUTOR_DIR`    | yes      | The path to the root of the serverless distributor in its repo
`FEEDER_DISTRIBUTOR_CONFIG_DIR` | yes      | The path to the directory containing the shared `feeder` and `distribute` commands' config file
`FEEDER_DISTRIBUTOR_CONFIG_FILE`| yes      | The name of the shared `feeder` and `distrubute` commands' config file in `${FEEDER_DISTRIBUTOR_CONFIG_DIR}`
`DISTRIBUTOR_GITHUB_TOKEN`      | yes      | A GitHub [Personal Access Token](https://docs.github.com/en/github/authenticating-to-github/keeping-your-account-and-data-secure/creating-a-personal-access-token) for the `distribute` command to use to create a PR on the distributor repo with the witnessed checkpoint
`GIT_USERNAME`                  | yes      | The GitHub username associated with the Personal Access Token
`GIT_EMAIL`                     | yes      | An email address to associate with the feeder commits
`WITNESS_PRIVATE_KEY`           | yes      | The witness private key in note format (*not* the path to the key)
`WITNESS_CONFIG_DIR`            | no       | Absolute path to a directory containing a witness config file (default: `/etc/witness/config`)
`WITNESS_CONFIG_FILE`           | no       | The name of the witness config file within `${WITNESS_CONFIG_DIR}` (default: `witness.config`)
`INTERVAL_SECONDS`              | no   | The number of seconds between feed/witness/distribute attempts, set to empty string for one-shot (default: 300s)

With the env variables configured, the witness can be started with the following command:

```bash
$ docker-compose -f serverless/deploy/docker/witness/docker-compose.yaml up
Starting witness_witness_1 ... done
Attaching to witness_witness_1
witness_1  | I0714 18:20:19.300495       1 witness.go:88] Connecting to local DB at "/data/witness.sqlite"
witness_1  | I0714 18:20:19.301276       1 witness.go:108] Starting witness server...
```

> :frog:
>
> You can store the config files necessary for running this feeder in a directory somewhere other
> than in the repo itself by using the `docker-compose` `--env-file` flag, along with absolute paths
> for the `..._CONFIG_DIR` variables in your `.env` file.
>
> The example below uses the `${HOME}/data` directory for storing these config files:
> 
> ```bash
> $ cd ${HOME}/data
> $ find .
> ./.env
> ./distributor
> ./distributor/distributor.config
> ./witness
> ./witness/witness.config
> ```
> 
> and populate your `${HOME}/data/.env` file like so:
> ```yaml
> SERVERLESS_DISTRIBUTOR_REPO=AlCutter/serverless-test
> SERVERLESS_DISTRIBUTOR_FORK=<YourGitHubUserName>/serverless-test
> SERVERLESS_DISTRIBUTOR_DIR=distributor
> DISTRIBUTOR_CONFIG_DIR=/home/<your username>/data/feeder
> DISTRIBUTOR_CONFIG_FILE=feeder.yaml
> DISTRIBUTOR_GITHUB_TOKEN=<YourGitHubPersonalAccessToken>
> GIT_USERNAME=<YourGitHubUserName>
> GIT_EMAIL=<your@email>
> WITNESS_PRIVATE_KEY=<your witness private key>
> WITNESS_CONFIG_DIR=/home/<your username>/data/witness
> WITNESS_CONFIG_FILE=witness.config
> INTERVAL_SECONDS=300s
> ```
>
> Start the feeder from within your `${HOME}/data` directory like so:
> ```bash
> $ cd ${HOME}/data
> $ docker-compose -f ../trillian/trillian-examples/serverless/deploy/docker/witness/docker-compose.yaml --env-file=./.env up
> ```