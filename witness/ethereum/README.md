# Ethereum Witness

## Run tests

### with docker

- cd into this directory
- `docker run --rm -it $(docker build -q .)`
  - This builds the container, runs it, and removes it after the run is
    complete.

### without docker (the hard way)

#### Install `pipenv` and `npm`

```bash
sudo apt install pipenv nodejs npm
```

#### Install `ganache-cli`

- Instructions from [here](https://github.com/trufflesuite/ganache-cli); run
  the following command:

```bash
npm install --global ganache-cli
```

#### Install `eth-brownie`

- Since our project uses `pipenv` to manage its python environment, we can
  just run the following:

```bash
pipenv install
```

(run from this directory)

#### Finally, run the tests:

```bash
pipenv run brownie test
```

## Explanation

An Ethereum Witness contract acts as an on-chain registry of Merkle tree
log states that are otherwise maintained off-chain. The contract handles
updates to logs and helps ensure that log operators cannot provide a split
view of the log prior to a confirmed checkpoint.

There are two witness contracts implementing different methodologies of
maintaining and updating the state of its tracked logs.
- The `RootWitness` contract maintains only the current roots of its tracked
logs, and updates them using full consistency proofs
- The `CompactRangeWitness` contract maintains compact ranges that represent
its tracked logs, and it updates these logs by merging in new compact ranges.
