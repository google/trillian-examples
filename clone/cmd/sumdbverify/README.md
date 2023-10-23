# `sumdbverify`

This tool clones the log for sum.golang.org and ensures that:
 1. The cloned data matches the commitments made in the log checkpoint
 2. That no module + version is declared with different hashes in the log

## Background

This is a quick summary of https://blog.golang.org/module-mirror-launch but is not
intended to replace this introduction.
If you have no context on Go SumDB, read that intro first :-)

Go SumDB is a Verifiable Log based on Trillian, which contains entries of the form:
```
github.com/google/trillian v1.3.11 h1:pPzJPkK06mvXId1LHEAJxIegGgHzzp/FUnycPYfoCMI=
github.com/google/trillian v1.3.11/go.mod h1:0tPraVHrSDkA3BO6vKX67zgLXs6SsOAbHEivX+9mPgw=
```
Every module & version used in the Go ecosystem will have such an entry in this log,
and the values are hashes which commit to the state of the repository and its `go.mod`
file at that particular version.

Clients can be assured that they have downloaded the same version of a module as
everybody else provided all of the following are true:
 1. The hash of what they have downloaded matches an entry in the SumDB Log
 2. There is only one entry in the Log for the `module@version`
 3. Entries in the Log are immutable / the Log is append-only
 4. Everyone else sees the same Log as this user

Verification of these is performed by different parties:
 1. The client checks an inclusion proof for their {module, version, hashes} in the Log
 2. This `sumdbverify` tool verifies there are no conflicting versions
 3. A [witness](https://github.com/transparency-dev/witness) verifies the log is append-only
 4. A [distributor](https://github.com/transparency-dev/distributor) aggregates multiple witness confirmations to verify consensus

## Running in Docker

The `docker-compose` scripts in this directory allow for deployment of the `sumdbclone` tool in a single command:

```bash
docker compose up -d
```

This will bring up the following containers:
 * `sumdbverify-db-1`: the database containing a local clone of the log
 * `sumdbverify-clone-1`: a service that periodically syncs the log into the DB
 * `sumdbverify-verify-1` a service that periodically reads the DB to verify the contents

The clone tool will initially run in a batch mode to download all of the entries.
To see the status of this, use the following command:

```bash
docker logs sumdbverify-clone-1 -f

I0404 10:23:13.018137       1 clone.go:177] 18636.3 leaves/s, last leaf=4700160 (remaining: 12130037, ETA: 10m50s), time working=97.1%
```

Once the initial batch clone is complete, the tool will periodically poll the log and clone any new entries.

Any errors in verifying the contents of the log will show up as crash looping of `sumdbverify-verify-1`.
Inspecting the `docker logs` for this process will reveal the cause of the errors.
