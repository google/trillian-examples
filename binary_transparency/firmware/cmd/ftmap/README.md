Verifiable Maps
===============

The verifiable map in this firmware transparency demonstrates how annotations (also known as attestations) about a piece of firmware can be verifiably presented to clients.
The naive way of doing this would be to simply give the client inclusion proofs to the annotations in the log.
The problem with this approach is that there is no way to prove to the client that they have been presented *all* relevant annotations; logs do not support non-inclusion proofs.

The map is constructed *entirely from the log*, which means that the state of a map at a given size depends only on:
 * The number of entries it consumes from the log
 * The map and reduce functions that are applied to convert the log entries into map entries

Map Checkpoints contain the Log Checkpoint they were constructed from, along with the number of entries the map consumed.
The functions used in the map are public knowledge.
These two facts mean that anybody with sufficient computing power to process the log can verify the map state by running the same map building calculation, and comparing root hashes.

The map in this demonstration creates two types of entry in the map:
 * A log of all firmware for each device
 * For each logged piece of firmware, all annotations for it are aggregated together

The first of these is a proof of concept and isn't currently read by clients (TODO(mhutchinson): remove this if needed for simplicity).
The second of these is used as an additional check when flashing firmware to check that no scanners have found malware in it, if the `map_url` argument is provided to the flash tool.

TODO(mhutchinson): make the map and reduce functions super clear in the code and refer to them from here.

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
go run ./cmd/ft_monitor/ --logtostderr --v=1 --keyword="H4x0r3d" --state_file=/tmp/ftmon.state --annotate
```

Now generate the map from the log, storing the resulting data in a sqlite DB at ~/ftmap.db:

* `go run ./cmd/ftmap --alsologtostderr --v=2 --runner=universal --endpoint=localhost:8099 --environment_type=LOOPBACK --map_db ~/ftmap.db --trillian_mysql="test:zaphod@tcp(127.0.0.1:3336)/test"`

The map server can now be run to serve from this DB:

* `go run ./cmd/ftmapserver --map_db ~/ftmap.db --alsologtostderr --v=1 &`

The map server will now be running at `localhost:8001`. We can point the flash tool at this server to perform additional checks by passing `--map_url=http://localhost:8001` when flashing to the device, e.g:

* `go run ./cmd/flash_tool/ --logtostderr --update_file=/tmp/update.ota --device_storage=/tmp/dummy_device --device=dummy --map_url=http://localhost:8001`

After performing all of the other checks, this will verifiably read the aggregated findings for the candidate firmware from the map and check that no malware has been reported for it.
