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
