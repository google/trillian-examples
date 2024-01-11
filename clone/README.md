# Log Cloner

This directory contains a library, database, and tools for cloning transparency logs.
The core library and database is log-agnostic, and each tool tailors this generic library to a specific log.

## Design Considerations

The core library attempts to balance optimization of the following goals:
  1. Downloading as quickly as possible
  2. Backing off when requested by the log (i.e. not DoSing the log)
  3. Simple local state / recovery
  4. Reusability across all verifiable logs

This is achieved by:
  1. Downloading batches of leaves in parallel
  2. Using exponential backoff on transient failures
  3. Writing leaves to the local database strictly in sequence
     1. This ensures there are no missing ranges, which keeps state tracking easier
  4. Treating all leaf data as binary blobs, with no attempt to parse them

## How it Works

The goal is to download data as quickly as possible from the log, and then persist verified data locally.
A single download session looks like this:

1. Get a checkpoint from the log and store this _in memory_
   1. We will refer to the size of this checkpoint (i.e. the number of leaves it commits to) as `N`
2. Read the last checkpoint persisted in the local database in order to determine the local size, `M`
   1. If no previous checkpoint is stored then `M` is 0
3. Download all leaves in the range `[M, N)` from the log
   1. Leaves are fetched in batches, in parallel, and pooled in memory temporarily
   2. Leaves from this memory pool are written to the `leaves` table of the database from this memory pool, strictly _in order_ of their index
4. Once `N` leaves have been written to the database, calculate the Merkle root of all of these leaves
5. If, and only if, the Merkle root matches the checkpoint downloaded in (1), write this checkpoint to the `checkpoints` table of the database
   1. A compact respresentation of the Merkle tree is also stored along with this checkpoint in the form of a [compact range](https://github.com/transparency-dev/merkle/tree/main/compact)

Note that this means that until a download session completes successfully, the database may contain unverified leaves with an index greater than that stored in the latest checkpoint.
Leaves must not be trusted if their index is greater or equal to the size of the latest checkpoint.

## Custom Processing

This library was designed to form the first part of a local data pipeline, i.e. downstream tooling can be written that reads from this local mirror of the log.
Such tooling MUST only trust leaves that are committed to by a checkpoint; reading leaves with an index greater than the current checkpoint size is possible, but such data is unverified and using this defeats the purpose of using verifiable data structures.

The `leaves` table records the leaf data as blobs; this accurately reflects what the log has committed to, but does not enable efficient SQL queries into the data.
A common usage pattern for a specific log ecosystem would be to have the first stage of the local pipeline parse the leaf data and break out the contents into a table with an appropriate schema for the parsed data.

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
  * [serverless HTTP logs](cmd/serverlessclone/)
 
See the documentation for these for specifics of each tool.

## Using Cloned Data

The data is stored in a simple database table named `leaves`, where each leaf in the log
is identified by its position in the log (column `id`) and the data at that position is a
blob (column `data`). The expectation is that clients will write custom tooling that reads
from this SQL table in order to perform custom tasks, e.g. verification of data, searching
to find relevant records, etc.

An example query that returns the first 5 leaves in the DB:
```
select * from leaves where id < 5;
```

## Quick Start (Mac)

On a Mac with [Homebrew](https://brew.sh) already installed, getting MariaDB installed
and connected to it in order to run the setup above is simple.
At the time of writing, this installed `10.8.3-MariaDB Homebrew` which works well for
the cloning tool.

```
brew install mariadb
brew services restart mariadb
mysql
```
