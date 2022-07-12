# The OmniWitness

The OmniWitness is a witness that will monitor all [known](./feeder_configs/) logs that use
the [generic checkpoint format](https://github.com/transparency-dev/formats/tree/main/log).

The OmniWitness is opinionated on which logs and distributors will be used. Deploying any
of the OmniWitness implementations will perform the same witnessing and distribution, unless
the operator changes any of the configuration files in the `*_configs` directories here.

This directory contains a dockerized implementation for a micro-services style deployment;
a container will be deployed for:
  * The core witness
  * Each of the feeder types (one for each of the config files in `./feeder_configs`)
  * Each of the distributors ([mhutchinson-distributor](https://github.com/mhutchinson/mhutchinson-distributor) and [WolseyBankWitness](https://github.com/WolseyBankWitness/rediffusion))

Instructions for deploying this are expanded below.

In addition, there are two other equivalent implementations with different deployments:
  * [monolith](./monolith): all in one process, can be run bare or in Docker
  * [usbarmory](./usbarmory): all in one unikernel, for specialized hardware

## Common Configuration

All deployments will need a keypair for signing/verifying witnessed checkpoints.
Most deployments should push the witnessed checkpoints to the distributors.
Instructions are provided for generating the material needed to do this.

Each deployment of the omniwitness will take these credentials in a different
way, so see the specific documentation for the deployment you are using.

Take care to protect any private material!

## Witness Key Generation

It is strongly recommended to generate new key material for a witness. This key
material should only be used to sign checkpoints that have been verified to be
an append-only evolution of any previously signed version (or TOFU in the case
of a brand new witness).

A keypair can be generated using `note.GenerateKey`; example code is provided
at https://play.golang.org/p/uWUKLNK6h9v. It is recommended to copy this code
to your local machine and run from there in order to minimize the risk to the
private key material.

## Github Credentials

This is optional, but recommended in order to push your checkpoints to as wide
an audience as possible and strengthen the network.
Checkpoints will be pushed to the distributors using GitHub pull requests.
This means that GitHub credentials must be provided.
It is strongly recommended to set up a secondary account for this purpose.

Github setup:
  * Create a personal access token with `repo` and `workflow` permissions at https://github.com/settings/tokens
  * Fork both of the repositories:
    * https://github.com/mhutchinson/mhutchinson-distributor
    * https://github.com/WolseyBankWitness/rediffusion

Raise PRs against the distributor repositories in order to register your new
witness in their configuration files. Documentation on how to do this is found
on the README at the root of these repositories.

# OmniWitness: Dockerized

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

`WITNESS_PRIVATE_KEY` and `WITNESS_PUBLIC_KEY` should be generated as documented above.

If you wish to use the distributors to push to GitHub, follow the configuration steps above and then:
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

