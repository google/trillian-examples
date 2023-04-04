# `sumdbclone`

This tool clones the log for sum.golang.org.
See background and database setup in the [parent docs](../../README.md).

## Cloning

Assuming the database is provisioned (with database called `sumdbclone`), the log can be downloaded with:

```
go run ./clone/cmd/sumdbclone --alsologtostderr --v=1 --mysql_uri 'clonetool:letmein@tcp(localhost)/sumdbclone'
```

## Docker

### SQL DB in Docker

The `docker-compose` scripts in this directory allow for deployment of the `sumdbclone` tool in a single command:

```bash
docker-compose up -d
```

This will bring up two containers: `sumdbclone_db_1` and `sumdbclone_clone_1`.
The clone tool will initially run in a batch mode to download all of the entries.
To see the status of this, use the following command:

```bash
docker logs sumdbclone_clone_1 -f

I0404 10:23:13.018137       1 clone.go:177] 18636.3 leaves/s, last leaf=4700160 (remaining: 12130037, ETA: 10m50s), time working=97.1%
```

Once the initial batch clone is complete, the tool will periodically poll the log and clone any new entries.
At this time, it is reasonable to start making queries of the database.
It is expected that users will write their own tools to query the DB, but the following command demonstrates the leaves being queried from the command line:

```bash
docker exec -i sumdbclone_db_1 /usr/bin/mysql -usumdb -pletmein -Dsumdb <<< "select * from leaves where id < 5;"
```

### External SQL DB

To build a docker image, run the following from the `trillian-examples` root directory:

```
docker build . -t sumdbclone -f ./clone/cmd/sumdbclone/Dockerfile
```

This can be pointed at a local MySQL instance running outside of docker using:

```
docker run --name clone_sumdb -d sumdbclone --alsologtostderr --v=1 --mysql_uri 'clonetool:letmein@tcp(host.docker.internal)/sumdbclone'
```
