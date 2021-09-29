# Ethereum Witness Feeder

## Explanation

The Ethereum Witness Feeder is a script used to periodically "feed" Trillian log data provided by public APIs to an Ethereum smart contract.

See the [Ethereum Witness README](https://github.com/google/trillian-examples/blob/master/witness/ethereum/README.md) for additional context.

## To run

- cd into this directory (i.e. `feeder/ethereum`)
- make sure your local environment matches the `.env.example` file in this directory, either via setting a local `.env` file in this directory, or manually passing in the env vars

### Continue with Docker

- `docker run --rm -it $(docker build -q .)`
  - This builds the container, runs it, and removes it after the run is complete.

### Continue without Docker

- make sure you have nodejs and yarn set up on your system
- run `yarn` to install dependencies
- run `yarn start` to run one iteration of the feeder
- run `yarn dev` to run the feeder in "watch" mode (useful for development)