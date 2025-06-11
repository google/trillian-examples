## Verifiable Index

Status: Experimental.

This idea has been distilled from years of experiments with maps, and a pressing need to have an efficient and verifiable way for an end-user to find _their_ data in logs without needing to download the whole log.

This experiment should be considered a 20% project for the time being and isn't on the near-term official roadmap for transparency.dev.
Discussions are welcome, please join us on [Transparency-Dev Slack](https://join.slack.com/t/transparency-dev/shared_invite/zt-27pkqo21d-okUFhur7YZ0rFoJVIOPznQ).

## Overview

The core idea is basically to construct an index like you would find in the back of a book, i.e. search terms are mapped to a _pointer_ to where the data can be found.
A verifiable index represents an efficient data structure to allow point lookups to common queries over a single log.
For example, a verifiable index over a module/package repository could be constructed to allow efficient lookup of all modules/packages with a given name.

The result of looking up a key in a verifiable index is a list of uint64 pointers to the origin log, i.e. a list of indices in the origin log where the leaf data matches the index function.
The index has a checkpoint that commits to its state at any particular log size.
Every point lookup (i.e. query) in the map is verifiable, as is the construction of the index itself.
The verifiable index commits to all evolutions of its state by committing to all published index roots in a witnessed output log.

## Applications

This verifiable map can be applied to any log where users have a need to enumerate all values matching a specific query. For example:

* CT: domain owners wish to query for all certs matching a particular domain  
* SumDB: package owners want to find all releases for a given package

Indices exist for both ecosystems at the moment, but they aren’t verifiable.

## Core Idea; TL;DR

The Verifiable Index has 3 data structures involved (and is informally called a Map Sandwich, as the Map sits between two Logs):

1. The _Input Log_ that is to be indexed
2. The _Verifiable Index_ containing pointers back into the _Input Log_
3. The _Output Log_ that contains a list of all revisions of the map

The Input Log likely aready exists before the Verifiable Index is added, but the Output Log is new, and required in order to make the Verifiable Index historically verifiable.
For example, in Certificate Transparency, the Input Log could be any one of the CT Logs.
In order to make certificates in a log be efficiently looked up by domain, an operator can spin up Verifiable Index and a corresponding Output Log.
The Index would map domain names to indices in the Input Log where the cert is for this domain.

> [!TIP]
> Note that the map doesn't have a "signed map root", i.e. it has no direct analog for a Log Checkpoint.
> Instead, the state of a Verifiable Index is committed to by including its state as a leaf in the Output Log.

> [!NOTE]
> A Verifiable Index is constructed for a single Input Log.
> For ecosystems of multiple logs (e.g. CT), there will be as many Verifiable Indices as there are Input Logs.

### Constructing

1. Log data is consumed leaf by leaf  
2. Each log leaf is parsed using a [MapFn](#mapfn-specified-in-universal-language) that specifies all of the keys in the map to which this relates  
   1. e.g. for CT, this would be all of the domains that relate to a cert.  
   2. The raw domains are not output, but are hashed. If privacy is important, a VRF could be used here.  
3. The output from the MapFn stage represents a sequence of update operations to the map  
   1. This output stream can be serialized for data recovery (see [Write Ahead Map Transaction Log](#write-ahead-map-transaction-log))
4. The map is computed for these update operations and a root is calculated
5. The root hash is written as a new leaf into the Output Log, along with the checkpoint from the Input Log that was consumed to create this revision
6. The Output Log is witnessed, and an Output Log Checkpoint is made available with witness signatures

### Reading

Users looking up values in the map need to know about the MapFn in order to know what the correct key hash is for, e.g. `maps.google.com`.
The values returned for a verifiable point lookup under this key would be a list of `uint64` values that represent indices into the log.
To find the certs for these values, the original log is queried at these indices.

Given a key to read, a read operation needs to return:
 - A witnessed Output Log Checkpoint
 - The latest value in this log, with an inclusion proof to the Output Log Checkpoint
   - The value in this log commits to the Input Log state, and also contains a Verifiable Index root hash
 - The value at the given key in the Verifiable Index, and an inclusion proof

Verifying this involves verifying the following chain:
 - The Output Log Checkpoint is signed by the Map Operator, and sufficient witnesses
 - The inclusion proof in the Output Log: this ties the Map Root Hash to the Output Log Checkpoint
 - The inclusion proof in the Verifiable Index: this ties the indices returned to the key and the Map Root Hash

### Verifying

The correct construction of the map can be verified by any other party.
The only requirement is compute resources to be able to build the map, and a clear understanding of the MapFn (hence the importance for this to be universally specified).
The verifier builds a map at the same size as the verifiable index and if the map checkpoint has the same root hash then both maps are equivalent and the map has been verified for correct construction.

## Sub-Problems

### MapFn Specified in Universal Language

Being able to specify which keys are relevant to any particular entry in the log is critical to allow a verifier to check for correct construction of the map. Ideally this MapFn would be specified in a universal way, to allow the verifier to be running a different technology stack than the core map operator. i.e. having the Go implementation be the specification is undesirable, as it puts a lot of tax on the verifier to reproduce this behaviour identically in another environment.

Some options:

* WASM ([go docs](https://go.dev/wiki/WebAssembly))  
* Formal spec  
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

> [!IMPORTANT]
> This describes the MapFn as returning key hashes.
> We _may_ want to have the map return the raw key (e.g. `maps.google.com`) so that a prefix trie can be constructed.

> [!IMPORTANT]
> The `MapFn` is fixed for the life of the Verifiable Index.
> There are strategies that could be employed to allow updates, but these are out of scope for any early drafts.

### Write Ahead Map Transaction Log

Having a compact append-only transaction log allows the map process to restart and pick up from where it last crashed efficiently. It also neatly divides the problem space: before this stage you have downloading logs and applying the MapFn, and after this stage you have the challenges of maintaining an efficient map data structure for updates and reads.

The core idea is to output at most a single record (row) for each entry in the log.
A valid row has:
 1. the first token being the log index (string representation of uint64)
 1. the following (optional) space-separated values being the key hashes under which this index should appear
 1. a newline terminator

Some use cases may have lots of entries in the log that do not map to any value, and so this supports omitting a log index if it has no updates required in the map.
However, an empty value can be output as a form of sentinel to provide a milepost on restarts that prevents going back over large numbers of empty entries from the log.

```
0 HEX_HASH_1 HEX_HASH_2
2 HEX_HASH_52
3 HEX_HASH_99
4
6 HEX_HASH_2
```

In the above example, there is no map value for the entries at index 1, 4, or 5 in the log.
It is undetermined whether index 7+ is present, and thus anyone replaying this log would need to assume that this needs to be recomputed starting from index 7.

### Turning Log into Efficient Map

The [WAL](#write-ahead-map-transaction-log) can be transformed directly into the data structures needed for serving lookups.
This is implemented using two data structures that are maintained in lockstep:
 - A Verifiable Prefix Trie based on [AKD](https://github.com/facebook/akd): https://github.com/FiloSottile/torchwood/tree/main/mpt; this maintains only the Merkle Tree
 - A standard Go map; this stores the actual data, i.e. it maps each key to the list of all relevant indices

Keys in the map are hashes, according to whatever strategy [MapFn](#mapfn-specified-in-universal-language) returns.
Values are an ordered list of indices.

> [!NOTE]
> The current mechanism for hashing the list of indices writes all values out as a single block, and hashes this with a single SHA256.
> An alternative would be to build a Merkle tree of these values.
> This would be slightly more complex conceptually, but could allow for incremental updating of values, and more proofs.


## Status

There is a basic end-to-end application written that currently only supports SumDB.
This application:
 - Reads a local [SumDB clone](https://github.com/google/trillian-examples/tree/master/clone/cmd/sumdbclone)
 - Builds a map, in the format described above
 - Serves a Lookup API via HTTP

Running it:
 1. Have a fully mirrored SumDB using `sumdbclone` (the easiest way is to use `docker compose up`, and wait)
 1. Run the verifiable index binary: `go run ./experimental/vindex/cmd --logDSN="sumdb:letmein@tcp(127.0.0.1:33006)/sumdb"  --walPath ~/sumdb.wal`

Providing the above works, you will have a web server hosting a form at http://localhost:8088.
This form takes a package name for SumDB (e.g. `github.com/transparency-dev/tessera`), and outputs all indices in the SumDB log corresponding to that package.
You will also have a WAL file at `~/sumdb.wal`, which will make future boots faster.

## Milestones

|  #  | Step                                                      | Status |
| :-: | --------------------------------------------------------- | :----: |
|  1  | Public code base and documentation for prototype          |   ✅   |
|  2  | Implementation of Merkle Radix Tree                       |   ✅   |
|  3  | Example written for mapping SumDB                         |   ✅   |
|  4  | Example written for mapping CT                            |   ⚠️   |
|  5  | Output log                                                |   ❌   |
|  6  | Proofs served on Lookup                                   |   ❌   |
|  7  | MapFn defined in WASM                                     |   ❌   |
|  8  | Proper repository for this code to live long-term         |   ❌   |
|  N  | Production ready                                          |   ❌   |

