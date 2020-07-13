# Tritter
This is a demo of a deployment of an auditable log using
[Trillian](http://github.com/google/trillian).

## Scenario
You are creating a deployment which allows authenticated employees at your
company to post to a public messaging platform (`Tritter`) under the company's
shared account. This platform doesn't support multi-login, so you have created
a proxy (`TritBot`) that takes requests from employees and posts these to
`Tritter`.

`TritBot` logs each message request with the details of the sender in order to
prevent abuse.

## Deployment 1: Non-Verifiable Storage
While these services can be run in the same terminal, for the sake of clarity
and preserving sanity, I recommend opening a terminal for each of the services
started.

* **Terminal 1: Tritter**

```
go run ./tritter/server/server.go -alsologtostderr
```

* **Terminal 2: File Logger**

```
go run ./tritbot/log/file/file.go -alsologtostderr
```

* **Terminal 3: TritBot**

```
go run ./tritbot/client/client.go -alsologtostderr --logger_addr="localhost:50052" "This is the message to post"
```

You should see `This is the message to post` appear on both the `Tritter`
terminal, and in the log file (by default `/tmp/tritter.log`).

## Deployment 2: Verifiable Storage
While these services can be run in the same terminal, for the sake of clarity
and preserving sanity, I recommend opening a terminal for each of the services
started.

* **Terminal 1: Tritter**

```
go run ./tritter/server/server.go -alsologtostderr
```

* **Terminal 2: Trillian Services**

```
export GO111MODULE=on
go run github.com/google/trillian/server/trillian_log_server --rpc_endpoint="localhost:50054" --http_endpoint="localhost:50055" &
go run github.com/google/trillian/server/trillian_log_signer --sequencer_interval="1s" --batch_size=500 --rpc_endpoint="localhost:50056" --http_endpoint="localhost:50057" --num_sequencers=1 --force_master &
# Trillian services are now running
go run github.com/google/trillian/cmd/createtree --admin_server=localhost:50054 # Take note of this tree/log ID

export TREE_ID= # Use the tree ID returned by createtree above
go run github.com/google/trillian/cmd/get_tree_public_key --admin_server=localhost:50054 --log_id=${TREE_ID}
# Put the public key returned by this into `tritbot/log/config.go`
```

TODO(mhutchinson): read the public key from file instead of hard-coding it into
the files

* **Terminal 3: Trillian Logger Personality**

```
export TREE_ID= # Use the tree ID returned by createtree above
go run ./tritbot/log/trillian/trillian.go --tree_id=${TREE_ID} -alsologtostderr
```

* **Terminal 4: TritBot**

```
go run ./tritbot/client/client.go -alsologtostderr --logger_addr="localhost:50053" "This is the audited message to post"
```

You should see `This is the message to post` appear on the `Tritter` terminal.
It can also be found in the Trillian DB. TODO(mhutchinson): examples of
querying the DB.

* **Terminal 5: Auditor**

```
go run ./tritbot/audit -alsologtostderr -v=2
```
