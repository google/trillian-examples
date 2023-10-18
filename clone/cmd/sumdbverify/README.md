# `sumdbverify`

This tool clones the log for sum.golang.org and ensures that:
 1. The cloned data matches the commitments made in the log checkpoint
 2. That no module + version is declared with different hashes in the log

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
