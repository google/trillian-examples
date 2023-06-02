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

### Docker Compose

`docker-compose.yaml` is provided as an example that will clone a log into a database hosted in a local container.
The benefit of this is that it can be run as a single command:

```bash
docker compose up -d
```

For Raspberry Pi users, there is a slight change in order to override the DB:

```bash
docker compose -f docker-compose.yaml -f docker-compose.rpi.yaml up -d
```

This will bring up two containers: `ctclone-db-1` and `ctclone-clone-xenon2022-1`.
It is expected that users will write their own tools to query the DB, but the following command demonstrates the leaves being queried from the command line:

```bash
docker exec -i ctclone-db-1 /usr/bin/mysql -uctclone -pletmein -Dxenon2022 <<< "select * from leaves where id < 5;"
```

The sample files here clone xenon2022, though this can be changed to another log by updating the `docker-compose.yaml` and the `.env` file.
For users that would like to clone multiple logs doing this, creating multiple databases inside the db container is possible but [more complicated](https://stackoverflow.com/questions/39204142/docker-compose-with-multiple-databases).

### Direct

To build a docker image, run the following from the `trillian-examples` root directory:

```
docker build . -t ctclone -f ./clone/cmd/ctclone/Dockerfile
```

This can be pointed at a local MySQL instance running outside of docker using:

```
docker run --name clone_xenon2022 -d ctclone --alsologtostderr --v=1 --log_url https://ct.googleapis.com/logs/xenon2022/ --mysql_uri 'clonetool:letmein@tcp(host.docker.internal)/google_xenon2022'
```
