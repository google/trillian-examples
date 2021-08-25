# Log Cloner

This directory contains a library, database, and tools for cloning transparency logs.
The core library and database is log-agnostic, and each tool tailors this generic library to a specific log.

The core library attempts to balance optimization of the following goals:
  1. Downloading as quickly as possible
  2. Backing off when requested by the log (i.e. not DoSing the log)
  3. Simple local state / recovery

This is achieved by:
  1. Downloading batches of leaves in parallel
  2. Using exponential backoff on transient failures
  3. Writing leaves to the local database strictly in sequence
     1. This ensures there are no missing ranges, which keeps state tracking easier

These tools are written to clone the log at a point in time.
The first step is to download a checkpoint from the log, and then attempt to download all the leaves committed to by that checkpoint.
Once all the leaves have been downloaded, the Merkle tree is computed for the downloaded leaves and compared against the checkpoint.
If this verification succeeds, the checkpoint is persisted to a table in the database along with a compact range.
The compact range allows further runs of the tool to quickly synthesize the Merkle structure of the existing leaves, which makes incremental verification much faster.

This is designed such that downstream tooling can be written that reads from this local mirror of the log.
Such tooling should only trust leaves that are committed to by a checkpoint; checkpoints are only written after verification, where leaves are written blindly and verified afterwards.

## Database Setup

In MariaDB, create a database and user. Below is an example of doing this for MariaDB 10.6, creating a database `google_xenon2022`, with a user `clonetool` with password `letmein`.

```
MariaDB [(none)]> CREATE DATABASE google_xenon2022;
MariaDB [(none)]> CREATE USER 'clonetool'@localhost IDENTIFIED BY 'letmein';
MariaDB [(none)]> GRANT ALL PRIVILEGES ON google_xenon2022.* TO 'clonetool'@localhost;
MariaDB [(none)]> FLUSH PRIVILEGES;
```

## Tuning

The clone library logs information as it runs using `glog`.
Providing `--alsologtostderr` is passed to any tool using the library, you should see output such as the following during the cloning process:

```
I0824 11:09:23.517796 2881011 clone.go:71] Fetching [4459168, 95054738): Remote leaves: 95054738. Local leaves: 4459168 (0 verified).
I0824 11:09:28.519257 2881011 clone.go:177] 1202.8 leaves/s, last leaf=4459168 (remaining: 90595569, ETA: 20h55m21s), time working=24.2%
I0824 11:09:33.518444 2881011 clone.go:177] 1049.6 leaves/s, last leaf=4465183 (remaining: 90589554, ETA: 23h58m29s), time working=23.0%
I0824 11:09:38.518542 2881011 clone.go:177] 1024.0 leaves/s, last leaf=4470430 (remaining: 90584307, ETA: 24h34m21s), time working=23.3%
```

When tuning, it is recommended to also provide `--v=1` to see more verbose output.
In particular, this will allow you to see if the tool is encountering errors (such as being told to back off) by the log server, e.g. if you see lines such as `Retryable error getting data` in the output.

Assuming you aren't being rate limited, then optimization goes as follows:
  1. Get the `working` percentage regularly around 100%: this measures how much time is being spent writing to the database. To do this, increase the number of `workers` to ensure that data is always available to the database writer
  2. If `working %` is around 100, then increasing the DB write batch size will increase throughput, to a point

This process is somewhat iterative to find what works for your setup.
It depends on many variables such as log latency and rate limiting, the database, the machine running the clone tool, etc.

## Download Clients

Download clients are provided for:
  * [CT](cmd/ctclone/)
  * [sum.golang.org](cmd/sumdbclone/)
 
See the documentation for these for specifics of each tool.
