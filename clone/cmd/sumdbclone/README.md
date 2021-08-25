# `sumdbclone`

This tool clones the log for sum.golang.org.
See background and database setup in the [parent docs](../../README.md).

## Cloning

Assuming the database is provisioned (with database called `sumdbclone`), the log can be downloaded with:

```
go run ./clone/cmd/sumdbclone --alsologtostderr --v=1 --mysql_uri 'clonetool:letmein@tcp(localhost)/sumdbclone'
```

## Docker

To build a docker image, run the following from the `trillian-examples` root directory:

```
docker build . -t sumdbclone -f ./clone/cmd/sumdbclone/Dockerfile
```

This can be pointed at a local MySQL instance running outside of docker using:

```
docker run --name clone_sumdb -d sumdbclone --alsologtostderr --v=1 --mysql_uri 'clonetool:letmein@tcp(host.docker.internal)/sumdbclone'
```
