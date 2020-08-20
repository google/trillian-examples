# Auditor / Cloner for SumDB

The clone tool downloads all entries from the
[Go SumDB](https://blog.golang.org/module-mirror-launch) into a local SQLite
database, and verifies that the downloaded data matches the log commitment.

This auditor provides an example for how log data can be verifiably cloned, and
demonstrates how this can be used as a basis to verify its
[Claims](https://github.com/google/trillian/blob/master/docs/claimantmodel/).
In the Go SumDB case, the auditor needs to check that no module+version appears
in the log twice, as this would provide a vector for different clients to be
presented with different expected checksums. 

## Running

The following command will download all entries and store them in the database
file provided:
```bash
go run ./cli/clone/clone.go -db ~/sum.db
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

And the processed leaf data can be inspected to ensure that the same
module+version does not appear twice:
```bash
sqlite3 ~/sum.db 'SELECT module, version, COUNT(*) cnt FROM leafMetadata GROUP BY module, version HAVING cnt > 1;'
```

## TODO
* This only downloads complete tiles, which means that at any point there could
  be up to 2^height leaves missing from the database.
  These stragglers should be stored if the root hash checks out.
* The verified Checkpoint should be stored locally.
* Automatically check that no module & version appears twice in the log
  (the query to do this is provided above, but isn't yet performed by the tool).
* Only parse and process new leaves.
