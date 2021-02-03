# hello-world
This is a very basic example of how to use
[Trillian](http://github.com/google/trillian) to implement the operations
needed for a verifiable log.  It intentionally avoids
defining any interface between the client and personality, leaving only the
gRPC interface between the personality and the (Trillian) log.  In a real
deployment it would be necessary to define an interface between the client
and the personality, and thus implement a "real" client on top of the
current one.

In order to run the tests, it is necessary to first have a Trillian log
running, which itself means it is necessary to have MySQL or MariaDB
running (see [MySQL
setup](http://github.com/google/trillian#mysqlsetup) for more
information).  In one terminal, run the following commands:

```
export GO111MODULE=on
go run github.com/google/trillian/cmd/trillian_log_server --rpc_endpoint="localhost:50054" --http_endpoint="localhost:50055" &
go run github.com/google/trillian/cmd/trillian_log_signer --sequencer_interval="1s" --batch_size=500 --rpc_endpoint="localhost:50056" --http_endpoint="localhost:50057" --num_sequencers=1 --force_master &
# Trillian services are now running

export TREE_ID=$(go run github.com/google/trillian/cmd/createtree --admin_server=localhost:50054)
```

Now, in another terminal run `go build` and then `go test --tree_id=${TREE_ID}`.  
This runs three tests. 
1. `TestAppend` ensures that checkpoints update properly on the
personality side when new entries are appended to the log.
2. `TestUpdate` ensures that these updated checkpoints can be accepted by the 
client, after they are proved consistent with previous checkpoints.
3. `TestIncl` ensures that the client can be convinced about whether or not 
entries are in the log.
