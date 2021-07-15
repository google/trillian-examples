Witness
-------

This [docker-compose.yaml](docker-compose.yaml) file can be used to spin up a witness daemon.

You should set the following environmemt variables (either `export` or with a `.env` file):
Variable Name       | Description
------------------------------------------
WITNESS_CONFIG_DIR  | Absolute path to a directory containing a witness config file (default: `/etc/witness/config`)
WITNESS_CONFIG_FILE | The name of the witness config file within ${WITNESS_CONFIG_DIR} (default: `witness.config`)
WITNESS_PRIVATE_KEY | The witness private key in note format (*not* the path to the key)
SERVERLESS_LOG_REPO | The GitHub "<owner/repo>" fragment of the serverless log
SERVERLESS_LOG_FORK | The GitHub "<owner/repo>" fragment of the witness' fork of the serverless log
SERVERLESS_LOG_DIR  | The path to the root of the serverless log in its repo
FEEDER_CONFIG_DIR   | The path to the directory containing the `serverless/cmd/feeder` command's config file
FEEDER_CONFIG_FILE  | The name of the `serverless/cmd/feeder` command's config file in `${FEEDER_CONFIG_DIR}`
FEEDER_INTERVAL_SECONDS | The number of seconds between feed/witness attempts, set to empty string for one-shot
FEEDER_GITHUB_TOKEN | A GitHub Personal Access Token for the feeder to use to create a PR on the log repo with the witnessed checkpoint
GIT_USERNAME        | The GitHub username associated with the Personal Access Token
GIT_EMAIL           | An email address to associate with the feeder commits



The witness can be started with the following command:

```bash
$ docker-compose -f serverless/deploy/docker/witness/docker-compose.yaml up
Starting witness_witness_1 ... done
Attaching to witness_witness_1
witness_1  | I0714 18:20:19.300495       1 witness.go:88] Connecting to local DB at "/data/witness.sqlite"
witness_1  | I0714 18:20:19.301276       1 witness.go:108] Starting witness server...
```