Verifiable Maps
===============

This is a work in progress.

Preconditions
-------------

There are some pre-requisites to running this part of the demo:
 1. Have set up the environment and successfully run the [Trillian batchmap demo](https://github.com/google/trillian/tree/master/experimental/batchmap)
 2. Keep the Python Portable Beam Runner listening on port `8099`
 3. Have deployed the FT demo above using Docker, and have some firmware in there

Example Workflow
----------------

First add some good and bad firmware to the log, and then ensure the malware annotator has seen this and pushed its findings to the log.
The workflow script in the README in the `firmware` directory does this, but if you're short on time the following is a minimum set of client calls:

```bash
# Put a good piece of firmware in the log
go run ./cmd/publisher/publish.go --logtostderr --v=2 --timestamp="2021-10-10T15:30:20.19Z" --binary_path=./testdata/firmware/dummy_device/example.wasm --output_path=/tmp/update.ota --device=dummy

# Put a bad piece of firmware in the log
go run ./cmd/publisher/publish.go --logtostderr --v=2 --timestamp="2020-10-10T23:00:00.00Z" --binary_path=./testdata/firmware/dummy_device/hacked.wasm --output_path=/tmp/bad_update.ota --device=dummy

# Run the monitor to annotate
go run ./cmd/ft_monitor/ --logtostderr --v=1 --keyword="H4x0r3d" --annotate
```

Now to generate the map:

* `go run ./cmd/ftmap --alsologtostderr --v=2 --runner=universal --endpoint=localhost:8099 --environment_type=LOOPBACK --map_db ~/ftmap.db --trillian_mysql="test:zaphod@tcp(127.0.0.1:3336)/test"`

> :warning: this section still in progress

TODO(mhutchinson): serve the map tiles; clients need map inclusion proofs for this aggregated information.

It's possible to confirm that the aggregation has compiled the correct result by looking into the DB:
`sqlite3 ~/ftmap.db 'SELECT * FROM aggregations;'`
