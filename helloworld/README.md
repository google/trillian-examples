# helloworld
This is a basic example of how to use
[Trillian](http://github.com/google/trillian) to implement the operations
needed for a verifiable log.  This means providing instantiations for two
different actors:
- A **personality**, which acts a front-end for the (Trillian) log.  In CT,
  for example, the personality is CTFE.
- A **client**, which interfaces with the personality; in Trillian, the client
  should never talk to the log directly.  In CT, for example, the client might
  be an individual browser or a monitor.

This example intentionally avoids
defining any interface between the client and personality, leaving only the
gRPC interface between the personality and the (Trillian) log.  In a real
deployment it would be necessary to define an interface between the client
and the personality, for example a RESTful API or gRPC.

In order to run the tests, it is necessary to first have a Trillian log
running. Follow one of the two steps below:

## Manual

Set up MySQL or MariaDB using [MySQL setup](http://github.com/google/trillian#mysql-setup).
Then, in one terminal, run the following commands:

```bash
go run github.com/google/trillian/cmd/trillian_log_server --rpc_endpoint="localhost:50054" --http_endpoint="localhost:50055" &
go run github.com/google/trillian/cmd/trillian_log_signer --sequencer_interval="1s" --batch_size=500 --rpc_endpoint="localhost:50056" --http_endpoint="localhost:50057" --num_sequencers=1 --force_master &
# Trillian services are now running

export TREE_ID=$(go run github.com/google/trillian/cmd/createtree --admin_server=localhost:50054)
```

## Docker

```bash
docker compose up -d
# Wait a second for MySQL to start up between these commands
curl https://raw.githubusercontent.com/google/trillian/master/storage/mysql/schema/storage.sql | docker exec -i helloworld_db_1 mysql -pzaphod -Dtest
```

# Running the tests

After setting up the trillian environment using the Manual or Docker instructions:

Now, in another terminal run `go build` and then `go test --tree_id=${TREE_ID}`.  
This runs three tests. 
1. `TestAppend` ensures that checkpoints update properly on the
personality side when new entries are appended to the log.
2. `TestUpdate` ensures that these updated checkpoints can be accepted by the 
client, after they are proved consistent with previous checkpoints.
3. `TestIncl` ensures that the client can be convinced about whether or not 
entries are in the log.
