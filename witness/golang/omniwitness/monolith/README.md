# OmniWitness: Monolith

See the [parent directory](../) for an introduction to what the OmniWitness is, and for
common configuration steps such as key generation and creating github credentials.

This version uses a single `main` construct to invoke all of the sub-components needed to
implement the OmniWitness. 

## Running Bare

The configuration files referenced in the parent OmniWitness docs are compiled into the
application during build. The remaining configuration is that specific to the operator,
which is provided via flags (though see #662; this will change).

The simplest possible configuration brings up the OmniWitness to follow all of the logs,
but the witnessed checkpoints will not be distributed and can only be disovered via the
witness HTTP endpoints (see parent directories for documentation on these):

```
go run github.com/google/trillian-examples/witness/golang/omniwitness/monolith@master --alsologtostderr --v=1 \
  --private_key PRIVATE+KEY+my.witness+67890abc+xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx \
  --public_key my.witness+67890abc+xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx \
  --db_file ~/witness.db
```

A more advanced configuration for users that are committed to running the witness is to
set up witnessed checkpoints to be distributed via the GitHub distributors, which will
strengthen the ecosystem. Note that this requires more configuration of GitHub secrets,
and your witness key adding to the configuration files for the distributors. This
configuration is detailed in the parent directories.

```
go run github.com/google/trillian-examples/witness/golang/omniwitness/monolith@master --alsologtostderr --v=1 \
  --private_key PRIVATE+KEY+my.witness+67890abc+xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx \
  --public_key my.witness+67890abc+xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx \
  --gh_user my-github-user \
  --gh_email foo@example.com \
  --gh_token ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx \
  --db_file ~/witness.db
```

## Running in Docker

1. Copy the `docker-compose.yaml` file from this directory to your local filesystem, e.g. `/etc/omniwitness/docker-compose.yaml`
1. Create a `.env` file in the same directory, using the template described in the [parent README](../#configuration)
1. From that directory, run `docker-compose up -d`


