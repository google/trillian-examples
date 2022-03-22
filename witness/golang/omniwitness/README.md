# The OmniWitness

This is a docker configuration that allows a witness to be deployed that will witness checkpoints from all known logs
that have a valid note Checkpoint format (logs are explicitly listed in `./witness_configs/witness.yaml`).
Features:
 * The witness will expose port 8100 outside of the container
   * This can be exposed to other machines if you wish to use the witness as a distributor
 * Two redistributors will be started that will attempt to push checkpoints to
   GitHub distributors:
   * https://github.com/mhutchinson/mhutchinson-distributor
   * https://github.com/WolseyBankWitness/rediffusion

## Configuration

The only file that should need to be edited is the `.env` file which needs to be created in key-value format following this template:

```
WITNESS_PRIVATE_KEY=PRIVATE+KEY+YourTokenHere+XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
WITNESS_PUBLIC_KEY=YourTokenHere+01234567+AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA

GITHUB_AUTH_TOKEN=ghp_XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
GIT_USERNAME=johndoe
GIT_EMAIL=johndoe@example.com

WITNESS_VERSION=latest
```

`WITNESS_PRIVATE_KEY` and `WITNESS_PUBLIC_KEY` should be generated for this witness, and the private key kept secret.
The keys can be generated using `note.GenerateKey`; example code is provided https://play.golang.org/p/uWUKLNK6h9v.

If you wish to use the distributors to push to GitHub, then the other variables to set are those matching `GIT*`.
Checkpoints will be pushed to the distributors using GitHub pull requests.
This means that GitHub credentials must be provided.
If you aren't comfortable doing this on your primary account, set up a secondary account for this purpose.

Github setup:
  * Create a personal access token with `repo` and `workflow` permissions at https://github.com/settings/tokens
  * Fork both of the repositories:
    * https://github.com/mhutchinson/mhutchinson-distributor
    * https://github.com/WolseyBankWitness/rediffusion

Once this is done, modify the `.env` file:
  * The token should be set as `GITHUB_AUTH_TOKEN`
  * `GIT_USERNAME` is the GitHub user account
  * `GIT_EMAIL` is the email address associated with this account

## Running

From the directory containing the `docker-compose.yaml` file, run: `docker-compose up -d`.
Ensure that `docker ps -a` shows all of the services running.
If all is well then after a few minutes you should be able to see witnessed checkpoints locally:

```
# List all of the known logs
curl -i http://localhost:8100/witness/v0/logs
# Take a look at one of them
curl -i http://localhost:8100/witness/v0/logs/3e9617dce5730053cb82f0481b9d289cd3c384a9219ef5509c91aa60d214794e/checkpoint
```

If you set up the distributors correctly, then you should see pull requests being raised against the GitHub distributors.

If anything isn't working, the logs are the first place to look, e.g. `docker logs witness_sumdb.feeder_1`.
