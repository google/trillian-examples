# Lightweight Witness for SumDB

This directory contains a dockerized deployment for a lightweight witness and feeder.
Unlike the SumDB mirror & witness implementation in this repository, this version only acquires and stores a minimal amount of information.
This makes it much easier & cheaper to deploy.

The feeder will poll the SumDB periodically, and if the size of its checkpoint is larger than the one committed to by the witness, then the feeder will generate a consistency proof and send this to the witness.
The witness is simply a deployment of the [generic witness](../../witness/golang/README.md).

## Running

### Initial Setup

Before running the witness, first prepare a config directory containing `witness.config`.
If this is copied anywhere other than `/etc/witness/config/witness.config` then `WITNESS_CONFIG_DIR` and `WITNESS_CONFIG_FILE` environment variables must be set when running docker compose later.

You should also generate a keypair for the witness using `note.GenerateKey`, e.g. https://play.golang.org/p/uWUKLNK6h9v.

### Running The Witness

Change the environment variables below to the values you have generated, and set the config directory/file variables similarly if needed.

```bash
export WITNESS_PUBLIC_KEY="WitnessName+publickey"; \
export WITNESS_PRIVATE_KEY="PRIVATE+KEY+WitnessName+privatekey"; \
docker compose -f ./sumdbaudit/witness/docker-compose.yaml up -d
```

To confirm this is working you can run `curl localhost:8100/witness/v0/logs/sumdb/checkpoint` which should give something like:

```
go.sum database tree
6353345
d5kp6/RTbWj/e9AMga1hdcHZHcLdFKA2fjHfKJ8FfR4=

— sum.golang.org Az3grhfKc0hf9eAH1x5p0VjY99pEe8l9JKKLGyf9F0m4JTrJjcsr9rUDh6kvNIl7vdzWqULpk3+azvpfJo9aOMZaYQE=
— mdh.SumDB.Witness MwRoUvap0myxmJQ+D1EuU61mmID6Fu1anufFrU0E0FgqaVj8ZAjWSJ7eWzi8tQ4dOljZ2cVlDmSesoaeBAMC1t94Mgc
```
