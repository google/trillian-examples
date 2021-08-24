# `ctclone`

This tool clones a Certificate Transparency (RFC6962) log.
See background and database setup in the [parent docs](../../README.md).

## Cloning

Assuming the database is provisioned, the log can be downloaded with:

```
go run ./clone/cmd/ctclone --alsologtostderr --v=1 --log_url https://ct.googleapis.com/logs/xenon2022/ --mysql_uri 'clonetool:letmein@tcp(localhost)/google_xenon2022'
```

## Tuning

In addition the general tuning flags (`workers` and `write_batch_size`) mentioned in the parents docs, the CT clone tool also has `fetch_batch_size`. As a rule of thumb, this should be set to the maximum size that the log supports for a single batch.

## Docker

To build a docker image, run the following from the `trillian-examples` root directory:

```
docker build . -t ctclone -f ./clone/cmd/ctclone/Dockerfile
```

This can be pointed at a local MySQL instance running outside of docker using:

```
docker run --name clone_xenon2022 -d ctclone --alsologtostderr --v=1 --log_url https://ct.googleapis.com/logs/xenon2022/ --mysql_uri 'clonetool:letmein@tcp(host.docker.internal)/google_xenon2022'
```
