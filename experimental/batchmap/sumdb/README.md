# Verifiable Map for Go SumDB

This is a demo that generates a Verifiable Map for the
[Go SumDB](https://blog.golang.org/module-mirror-launch) to allow point lookups for module versions.
The keys in the map are the same format as found in `go.sum` files, e.g.
 * `github.com/google/trillian v1.3.11`
 * `github.com/google/trillian v1.3.11/go.mod`

The logical value in the map is the hash formatted as in the `go.sum` file, e.g. `h1:pPzJPkK06mvXId1LHEAJxIegGgHzzp/FUnycPYfoCMI=`.
This is not the literal value however; for proper cryptographic security the value is protected with extra levels of salting which are specific to the key and tree ID.
See the implementation of `mapEntryFn` in `build/map.go` for the implementation of the value construction.

## Running

There are some pre-requisites to running this demo:
 1. Have set up the environment and successfully run the [Trillian batchmap demo](https://github.com/google/trillian/tree/master/experimental/batchmap)
 2. Have run the `sumdbaudit` example and have the `sum.db` file it generates on your machine

Congratulations on getting this far.
Now, assuming the Python Portable Beam runner is listening on port `8099` the following will generate the verifiable map for every entry downloaded from the SumDB (assumed working directory is the one containg this README):

 * `go run build/map.go --output=/tmp/sumdbmap --sum_db=/path/to/sum.db --runner=universal --endpoint=localhost:8099 --environment_type=LOOPBACK`

This will put a file per tile in `/tmp/sumdbmap`.
The verifier can check that every entry in a `go.sum` file is properly committed to by the map:

 * `go run verify/verify.go --logtostderr --v=1 --map_dir=/tmp/sumdbmap --sum_file=/path/to/go.sum`
