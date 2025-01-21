## Verifiable Index

Status: Experimental.

This idea has been distilled from years of experiments with maps, and a pressing need to have an efficient way for an end-user to find data in logs without performing a full scan of all of the data.

This experiment should be considered a 20% project for the time being and isn't on the near-term official roadmap for transparency.dev.
Discussions are welcome, please join us on [Transparency-Dev Slack](https://join.slack.com/t/transparency-dev/shared_invite/zt-27pkqo21d-okUFhur7YZ0rFoJVIOPznQ).

## Overview

The core idea is basically to construct an index like you would find in the back of a book, i.e. search terms are mapped to a pointer to where the data can be found. A verifiable index represents an efficient data structure to allow point lookups to common queries over a single log. The result of looking up a key in a verifiable index is a uint64 pointer to the index in the origin log. The index has a checkpoint that commits to its state at any particular log size. Every point lookup (i.e. query) in the map is verifiable, as is the construction of the index itself.

## Applications

This verifiable map can be applied to any log where users have a need to enumerate all values matching a specific query. For example:

* CT: domain owners wish to query for all certs matching a particular domain  
* SumDB: package owners want to find all releases for a given package

These indices exist for both ecosystems at the moment, but they aren’t verifiable.

## Core Idea; TL;DR

### Constructing

1. Log data is consumed leaf by leaf  
2. Each log leaf is parsed using a [MapFn](#mapfn-specified-in-universal-language) that specifies all of the keys in the map to which this relates  
   1. e.g. for CT, this would be all of the domains that relate to a cert.  
   2. The raw domains are not output, but are hashed. If privacy is important, a VRF could be used here.  
3. The output from the MapFn stage represents a sequence of update operations to the map  
   1. This output stream can be serialized for data recovery (see [Write Ahead Map Transaction Log](#write-ahead-map-transaction-log))  
4. The map is computed for these update operations and is served to users

### Reading

Users looking up values in the map need to know about the MapFn in order to know what the correct key hash is for, e.g. `maps.google.com`. The values returned for a verifiable point lookup under this key would be a list of `uint64` values that represent indices into the log. To find the certs for these values, the original log is queried at these indices.

### Verifying

The correct construction of the map can be verified by any other party. The only requirement is compute resources to be able to build the map, and a clear understanding of the MapFn (hence the importance for this to be universally specified). The verifier builds a map at the same size as the verifiable index and if the map checkpoint has the same root hash then both maps are equivalent and the map has been verified for correct construction.

## Sub-Problems

### MapFn Specified in Universal Language {#mapfn-specified-in-universal-language}

Being able to specify which keys are relevant to any particular entry in the log is critical to allow a verifier to check for correct construction of the map. Ideally this MapFn would be specified in a universal way, to allow the verifier to be running a different technology stack than the core map operator. i.e. having the Go implementation be the specification is undesirable, as it puts a lot of tax on the verifier to reproduce this behaviour identically in another environment.

Some options:

* Formal spec  
* WASM ([go docs](https://go.dev/wiki/WebAssembly))  
* Functional language that can be transpiled

In any case, the MapFn functionally needs to be of the form:

```
type MapFn func([]byte) [][]byte
```

This would be used similarly to:

```
var i uint64 // the index currently being processed. Needs to be set to non-zero if log started mid-way through.
var log chan []byte // channel on which leaves from the log will be written
var mapFn MapFn // initialized somehow (maybe loading wasm)
var output func(i uint64, mapKeys ...[][]byte) // probably just writes the the write ahead log

for leaf := <- log {
  mapKeys := mapFn(leaf)
  output(i, mapKeys...)
  i++
}
```

i.e. consume each entry in the log, apply the map function in order to determine the keys to update, and then output this operation to the next stage of the pipeline.

### Write Ahead Map Transaction Log {#write-ahead-map-transaction-log}

Having a compact append-only transaction log allows the map process to restart and pick up from where it last crashed efficiently. It also neatly divides the problem space: before this stage you have downloading logs and applying the MapFn, and after this stage you have the challenges of maintaining an efficient map data structure for updates and reads.

The core idea is to output at most a single record (row) for each entry in the log. A row has the first token being the log index, and the following (optional) values being the key hashes under which this index should appear.

Some use cases may have lots of entries in the log that do not map to any value, and so this supports omitting a log index if it has no updates required in the map. However, an empty value can be output as a form of sentinel to provide a milepost on restarts that prevents going back over large numbers of empty entries from the log.

```
0 HEX_HASH_1 HEX_HASH_2
2 HEX_HASH_52
3 HEX_HASH_99
4
6 HEX_HASH_2
```

In the above example, there is no map value for the entries at index 1, 4, or 5 in the log. It is undetermined whether index 7+ is present, and thus anyone replaying this log would need to assume that this needs to be recomputed starting from index 7\.

### Turning Log into Efficient Map

And now we have the crux of the problem. The fun part 😀

Requirements:

* Values are indices, appended in incremental order. Could be a list or a log.

TODO(mhutchinson): Finish writing this up.

