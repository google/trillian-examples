# Auditor / Cloner for SumDB

This directory contains tools for verifiably creating a local copy of the
[Go SumDB](https://blog.golang.org/module-mirror-launch) into a local SQLite
database.
 * `cli/clone` is a one-shot tool to clone the Log at its current size
 * `cli/mirror` is a service which continually clones the Log
 * `cli/witness` is an HTTP service that uses a local clone of the Log to provide
   checkpoint validation for other clients. This is a very lightweight way of
   providing some Gossip solution to detect split views.

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
 * Committed entries are never modified; the Log is append-only
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

## Running `clone`

The following command will download all entries and store them in the database
file provided:
```bash
go run github.com/google/trillian-examples/sumdbaudit/cli/clone -sqlite_file ~/sum.db -alsologtostderr -v=2
```
This will take some time to complete on the first run. Latency and bandwidth
between the auditor and SumDB will be a large factor, but for illustrative
purposes this completes in around 4 minutes on a workstation with a good wired
connection, and in around 10 minutes on a Raspberry Pi connected over WiFi.
Your mileage may vary. At the time of this commit, SumDB contained a little over
1.5M entries which results in a SQLite file of around 650MB.

## Setting up a `mirror` service
These instructions show how to set up a mirror service to run on a Raspberry Pi
running a recent version of Raspbian.

> :frog: this would be more useful with a client/server database instead of sqlite!

Setup:
```bash
# Build the mirror and install it where it can be executed
go build ./sumdbaudit/cli/mirror
sudo mv mirror /usr/local/bin/sumdbmirror

# Create a user to run the service that has no login
sudo useradd -M sumdb
sudo usermod -L -s /bin/false sumdb

# Create a directory to store the sqlite database
sudo mkdir /var/cache/sumdb
sudo chown sumdb.sumdb /var/cache/sumdb
```
Define the service by creating the file `/etc/systemd/system/sumdbmirror.service` with contents:
```
[Unit]
Description=Go SumDB Mirror
After=network.target
[Service]
Type=simple
User=sumdb
ExecStart=/usr/local/bin/sumdbmirror -sqlite_file /var/cache/sumdb/mirror.db -alsologtostderr -v=1

[Install]
WantedBy=multi-user.target
```

Start the service and check its progress:
```bash
sudo systemctl daemon-reload
sudo systemctl start sumdbmirror

# Follow the latest log messages
journalctl -u sumdbmirror -f
```

When the mirror service is sleeping, you will be able to query the local database at
`/var/cache/sumdb/mirror.db` using the example queries in the next section.
At the time of writing this setup uses almost 600MB of storage for the database.

If you want to have the `leafMetadata` table populated then you can add an extra argument
to the service definition.
In the `ExecStart` line above, add `-unpack` and then restart the `sumdbmirror` service
(`sudo systemctl daemon-reload && sudo systemctl restart sumdbmirror`).
When it next updates tiles this table will be populated.
This will use more CPU and around 60% more disk.

## Setting up a `witness` service
This requires a local clone of the SumDB Log to be available. For this to be of any
real value, it should be running against a database which is regularly being updated
by the `mirror` service described above.

> :warning: The witness is missing features (outlined below) in order to be used in an
> untrusted environment. This witness implementation is useful only in a trusted domain
> where the correct operation of the witness is implicit. This precludes being run as
> a general service on the Web, but is still useful within a household or organization.

A client which successfully checks its checkpoints with a witness can ensure that if
there is a "split view" of the SumDB Log, then it is on the same side of the split as
the witness. If this witness is also verifying the claims of the log, then the client
is safe in relying on the data within (providing it trusts the verifer!).

The service can be started with the command (assuming `~/sum.db` is the database):
```bash
go run ./sumdbaudit/cli/witness -listen :8080 -sqlite_file ~/sum.db -v=1 -alsologtostderr
```

This can be set up as a Linux service in much the same way as the `mirror` above.

Once running, the server will be available for GET requests at the listen address
given as a commandline parameter.

Some example requests that can be made:
```bash
# Simply get the latest golden checkpoint
curl -i http://localhost:8080/golden

# Validate that the witness is consistent with the Checkpoint your go build tools are using
curl -i http://localhost:8080/checkConsistency/`base64 -i ~/go/pkg/sumdb/sum.golang.org/latest`

# Validate that the witness is consistent with the latest Checkpoint from the real Log
curl -i http://localhost:8080/checkConsistency/`curl https://sum.golang.org/latest | base64`
```

### Using Docker

The witness can be started along with the mirror using `docker-compose`.
The following command will mirror the log and provide a witness on port `8080` when the initial
sync completes:
```bash
docker-compose -f sumdbaudit/docker/docker-compose.yml up --build
```

### Using a client/server database

The instructions above are for setting this up using sqlite with its storage on the local filesystem.
To set this up using MariaDB, the database can be provisioned by logging into the instance as root user and running the following:

```bash
CREATE DATABASE sumdb;
CREATE USER 'sumdb'@localhost IDENTIFIED BY 'letmein';
GRANT ALL PRIVILEGES ON sumdb.* TO 'sumdb'@localhost;
FLUSH PRIVILEGES;
```

Once set up, change the `sqlite_file` flag above for `mysql_uri` with a connection string like `'sumdb:letmein@tcp(127.0.0.1:3306)/sumdb?parseTime=true'`.

## Querying the database
The number of leaves downloaded can be queried:
```bash
sqlite3 ~/sum.db 'SELECT COUNT(*) FROM leaves;'
```

And the tile hashes at different levels inspected:
```bash
sqlite3 ~/sum.db 'SELECT level, COUNT(*) FROM tiles GROUP BY level;'
```

The modules with the most versions:
```bash
sqlite3 ~/sum.db 'SELECT module, COUNT(*) cnt FROM leafMetadata GROUP BY module ORDER BY cnt DESC LIMIT 10;'
```

## Missing Features
* This only downloads complete tiles, which means that at any point there could
  be up to 2^height leaves missing from the database.
  These stragglers should be stored if the root hash checks out.
* Only parse and process new leaves.
* Witness should return detailed responses
  * In the event of an inconsistency, both Checkpoints notes should be serialized and returned
  * Consistency should return a proof that the tree is consistent with the witnesses Golden Checkpoint
