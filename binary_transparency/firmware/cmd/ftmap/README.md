Verifiable Maps
===============

This is a work in progress.

Generating a Verifiable Map
---------------------------

There are some pre-requisites to running this part of the demo:
 1. Have set up the environment and successfully run the [Trillian batchmap demo](https://github.com/google/trillian/tree/master/experimental/batchmap)
 2. Keep the Python Portable Beam Runner listening on port `8099`
 3. Have deployed the FT demo above using Docker, and have some firmware in there

The following command will generate a verifiable map:

* `go run ./cmd/ftmap --alsologtostderr --v=2 --runner=universal --endpoint=localhost:8099 --environment_type=LOOPBACK --map_db ~/ftmap.db --trillian_mysql="test:zaphod@tcp(127.0.0.1:3336)/test"`

TODO(mhutchinson): Enhance the demo to show how devices can use this map to be convinced of useful aggregations across the log.