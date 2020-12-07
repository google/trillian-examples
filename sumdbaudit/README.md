# Auditor / Cloner for SumDB

The clone tool downloads all entries from the
[Go SumDB](https://blog.golang.org/module-mirror-launch) into a local SQLite
database, and verifies that the downloaded data matches the log commitment.

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
 * The hash of what they have downloaded matches an entry in the SumDB Log
 * There is only one entry in the Log for the `module@version`
 * Entries in the Log are immutable / the Log is append-only
 * Everyone else sees the same Log

## Features
This auditor provides an example for how Log data can be verifiably cloned, and
demonstrates how this can be used as a basis to verify its
[Claims](https://github.com/google/trillian/blob/master/docs/claimantmodel/).
The Claims checked by this clone & audit tool are:
 * SumDB Checkpoints/STHs properly commit to all of the data in the Log
 * Committed entries are never modified
 * The Log is append-only
 * Each `module@version` appears at most once

In addition to verifying the above Claims, the tool populates a SQLite database
with the following tables:
 * `leaves`: raw entries from the Log
 * `tiles`: tiled subtrees of the Merkle Tree
 * `checkpoints`: a history of Log Checkpoints (aka STHs) that have been seen
 * `leafMetadata`: parsed data from the `leaves` table

This tool does **not** check any of the following:
 * Everyone else sees the same Log: this requires some kind of Gossip protocol for
   clients and verifiers to share Checkpoints
 * That the hashes in the log represent the current state of the repository
   (the repository could have changed its git tags such that the hashes no longer
   match, but this is not verified)
 * That any `module@version` is "safe" (i.e. no checking for CVEs, etc)

## Running

The following command will download all entries and store them in the database
file provided:
```bash
go run github.com/google/trillian-examples/sumdbaudit/cli/clone -db ~/sum.db -alsologtostderr -v=2
```
This will take some time to complete on the first run. Latency and bandwidth
between the auditor and SumDB will be a large factor, but for illustrative
purposes this completes in around 4 minutes on a workstation with a good wired
connection, and in around 10 minutes on a Raspberry Pi connected over WiFi.
Your mileage may vary. At the time of this commit, SumDB contained a little over
1.5M entries which results in a SQLite file of around 650MB.

The number of leaves downloaded can be queried:
```bash
sqlite3 ~/sum.db 'SELECT COUNT(*) FROM leaves;'
```

And the tile hashes at different levels inspected:
```bash
sqlite3 ~/sum.db 'SELECT level, COUNT(*) FROM tiles GROUP BY level;'
```

## TODO
* This only downloads complete tiles, which means that at any point there could
  be up to 2^height leaves missing from the database.
  These stragglers should be stored if the root hash checks out.
* Only parse and process new leaves.
