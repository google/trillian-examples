# Verifiable Map for Go SumDB

## Intro & Motivation

This example shows how to build a Verifiable Map using the **experimental** Beam library in Trillian.

The data is sourced from a local clone of the Verifiable Log used to safeguard Go Modules, the
[Go SumDB](https://blog.golang.org/module-mirror-launch).
Entries in this Log are of the form:

```
github.com/google/trillian v1.3.11 h1:pPzJPkK06mvXId1LHEAJxIegGgHzzp/FUnycPYfoCMI=
github.com/google/trillian v1.3.11/go.mod h1:0tPraVHrSDkA3BO6vKX67zgLXs6SsOAbHEivX+9mPgw=
```

This data can be seen, along with the leaf index number and a Log Checkpoint (STH) at https://sum.golang.org/lookup/github.com/google/trillian@v1.3.11.
Clients with this leaf data, the Log Checkpoint, and a path to the root of the log can verify that this entry definitely is included within the log.
So we're done, right?

### Remaining Trust
Let's reframe the security goal of the log through the eyes of a client.
What a client wants to believe when they find an entry committed to by the log is that *all clients will see the same hashes for the same module@version*.

Confirming that the entry is committed to by the log only meets this goal if the log does not have conflicting information in it.
If the log ever contained entries that provided different hashes for the same module@version then the verification would still check out, but the security goal would not be met.

N.B. This is theoretical and has not happened; the whole of the SumDB log has been verified for duplicate entries and none have been found.
A tool to perform this checking is provided in this repo: https://github.com/google/trillian-examples/tree/master/sumdbaudit, so go look for yourself if you're now worried!

### Onto Maps

Verifiable Maps are a far better data structure for committing to data that is logically key+value.
A Map has only one value under a given key, and this value can be empty (thus it can even prove that there is no value at the key).

In this scenario the key is the module@version, and the value is the hash.
There is actually a separate part of the key that has not been discussed, which is what is being hashed.
Thus for each entry in the Go SumDB Log, there are two logical keys, e.g.
 * `github.com/google/trillian v1.3.11` - the hash of the zipped repository
 * `github.com/google/trillian v1.3.11/go.mod` - the hash of the `go.mod` file

The logical value in the map is the hash formatted as in the `go.sum` file, e.g. `h1:pPzJPkK06mvXId1LHEAJxIegGgHzzp/FUnycPYfoCMI=`.
This is not the literal value however; for proper cryptographic security the value is protected with extra levels of salting which are specific to the key and tree ID.
See the implementation of `mapEntryFn` in `build/map.go` for the implementation of the value construction.

A client that verifies inclusion of a key in a Verifiable Map can thus be satisfied that every client with the same map root will see the same hashes for any key they look up.

## Running

### Building

There are some pre-requisites to running this demo:
 1. Have set up the environment and successfully run the [Trillian batchmap demo](https://github.com/google/trillian/tree/master/experimental/batchmap)
 2. Have run the [sumdbaudit](https://github.com/google/trillian-examples/tree/master/sumdbaudit) example and have the `sum.db` file it generates on your machine; this mirror of the log is used as the input to the map

Congratulations on getting this far.
Now, assuming the Python Portable Beam runner is listening on port `8099` the following will generate the verifiable map for every entry downloaded from the SumDB (assumed working directory is the one containg this README):

 * `go run build/map.go --alsologtostderr --v=1 --runner=universal --endpoint=localhost:8099 --environment_type=LOOPBACK --sum_db=/path/to/sum.db --map_db=/path/to/map.db --count=256`

This will create a sqlite database at `/path/to/map.db` and store key/values for the first 256 entries from the SumDB log.
Note that this will actually create 512 entries in the map, as each entry in the log has 2 key+value pairs.

Remove the `count` parameter to process every entry, though you might want to do this while you make a nice cup of tea.

### Verifying

The verifier can check that every entry in a `go.sum` file is properly committed to by the map:

 * `go run verify/verify.go --alsologtostderr --v=1 --map_db=/path/to/map.db --sum_file=/path/to/go.sum`

### Updating

Once the map has been generated, it can be incrementally updated instead of generating the whole thing from scratch each time.
This is purely an optimization for large maps being updated with small deltas, and the resulting map will be the same whichever method is chosen to generate it.
Incremental update can be triggered by adding `--incremental_update` to the `build/map.go` arguments.
