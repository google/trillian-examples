# Log Cloner

A log client that copies a CT log to a local database for processing.
The tool downloads batches of leaves in parallel, but always writes them to the local database in sequence.
This ensures there are no missing ranges, which keeps state tracking easier.

One important implementation feature is that the download tools will exponentially back off if there are errors
communicating with the log; this prevents the client from performing a DoS on any log it is downloading.

## Setup

In MariaDB, create a database and user:

```
MariaDB [(none)]> CREATE DATABASE google_xenon2022;
MariaDB [(none)]> CREATE USER 'clonetool'@localhost IDENTIFIED BY 'letmein';
MariaDB [(none)]> GRANT ALL PRIVILEGES ON google_xenon2022.* TO 'clonetool'@localhost;
MariaDB [(none)]> FLUSH PRIVILEGES;
```

Now you can clone the log with:

```
go run ./clone/cmd/ctclone --alsologtostderr --v=1 --log_url https://ct.googleapis.com/logs/xenon2022/ --mysql_uri 'clonetool:letmein@tcp(localhost)/google_xenon2022'
```

See the optional flags in the `ctclone` tool to configure tuning parameters.
## Docker

To build a docker image, run the following from the `trillian-examples` root directory:

```
docker build . -t ctclone -f ./clone/cmd/ctclone/Dockerfile
```

This can be pointed at a local MySQL instance running outside of docker using:

```
docker run --name clone_xenon2022 -d ctclone --alsologtostderr --v=1 --log_url https://ct.googleapis.com/logs/xenon2022/ --mysql_uri 'clonetool:letmein@tcp(host.docker.internal)/google_xenon2022'
```

# Verification

The downloaded log contents should be checked against the checkpoints provided by the log.
There isn't yet any support in the DB for storing checkpoints, so maintaining them is a DIY project for now.
The `ctverify` tool takes a CT STH and uses its tree size to compute the root hash from the local data.
The root hashes are compared, and the tool exits with a failure if they do not match.

One way to accomplish this could be:

```bash
# 1. Get the checkpoint
wget -O xenon2022.checkpoint https://ct.googleapis.com/logs/xenon2022/ct/v1/get-sth
# 2. Clone the log as above ...
# 3. Pipe the original checkpoint into verify tool to check the downloaded content matches
cat xenon2022.checkpoint | go run ./clone/cmd/ctverify --alsologtostderr --mysql_uri 'mirror:letmein@tcp(localhost)/google_xenon2022'
```
