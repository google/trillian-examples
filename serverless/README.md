Serverless Log
===============

This is some experimental tooling which allows for the maintenance and querying
of a log represented by tiles & files. These tools are built upon components
from the Trillian repo (e.g. compact ranges), but *do not* use the Trillian Log
service itself.

The idea is to make logging infrastructure a bit more *nix like, and demonstrate
how to use those tools in conjunction with GitHub actions, GCP Cloud Functions,
AWS Lambda, etc. to deploy and maintain "serverless" transparency logs.

The on-disk structure of the log is well defined, and can directly be made
public via HTTP[S]. Clients wishing to be convinced of consistency/inclusion are
responsible for constructing the proofs themselves by fetching the tiles
containing the required nodes.

Command-line tools usage
------------------------

A few tools are provided for manipulating the on-disk log state:
 - `sequence` this assigns sequence numbers to new entries
 - `integrate` this integrates any as-yet un-integrated sequence numbers into
   the log state
 - `client` this provides log proof verification
 - `generate_keys` creates the public/private key pair for signing and
   validating the log checkpoints

Examples of how to use the tools are given below, they assume that a `${LOG_DIR}`
environment variable has been set to the desired path and directory name which
should contain the log state files, e.g.:

```bash
$ export LOG_DIR="/tmp/mylog"
```

`sequence` and `client` require the log public key to be provided.

This is supplied by providing the path to the key file using `--public_key` 
or by setting the `SERVERLESS_LOG_PUBLIC_KEY` environment variable

`integrate` requires the log public and private keys to be provided.

These are supplied by providing the path to the key files using 
`--public_key` and `--private_key` or by setting the
 `SERVERLESS_LOG_PUBLIC_KEY` and `SERVERLESS_LOG_PRIVATE_KEY` environment variables.


### Generating keys
To create a new private key pair, use the `generate_keys` command with `--key_name`, a name 
for the signing entity. You can output the public and private keys to files using   
`--out_pub` path and filename for the public key,     
`--out_priv` path and filename for the private key   
and stdout, private key, then public key, over 2 lines, using `--print`

```bash
$ go run ./serverless/cmd/generate_keys --key_name=astra --out_pub=key.pub --out_priv=key
```

### Creating a new log
To create a new log state directory, use the `integrate` command with the `--initialise`
flag, and either passing key files or with environment variables set:

```bash
$ go run ./serverless/cmd/integrate --initialise --storage_dir=${LOG_DIR} --logtostderr --public_key=key.pub --private_key=key
```

### Sequencing entries into a log
To add the contents of some files to a log, use the `sequence` command with the
`--entries` flag set to a filename glob of files to add and either passing the public key 
file or with the environment variable set:

```bash
$ go run ./serverless/cmd/sequence --storage_dir=${LOG_DIR} --entries '*.md' --logtostderr --public_key=key.pub
I0413 16:54:52.708433 4154632 main.go:97] 0: CONTRIBUTING.md
I0413 16:54:52.709114 4154632 main.go:97] 1: README.md
```

The tool prints out the names of added files, along with their assigned sequence
number(s) - above, the contents of `CONTRIBUTING.md` was assigned to sequence number
0.

Attempting to re-sequence the same file contents will result in the `sequence`
tool telling you that you're trying to add duplicate entries, along with their
originally assigned sequence numbers:

```
$ go run ./serverless/cmd/sequence --storage_dir=${LOG_DIR} --entries 'C*' --logtostderr --public_key=key.pub
I0413 16:58:08.956402 4155499 main.go:97] 0: CONTRIBUTING.md (dupe)
I0413 16:58:08.956938 4155499 main.go:97] 2: CONTRIBUTORS
```

Here we see that the contents of `CONTRIBUTING.md` already exists in the log at
sequence number 0, but the contents `CONTRIBUTORS` did not and was assigned a
sequence number of 2.

> :warning: </br>
> Note that duplicate suppression is not guaranteed - there are corner
> cases where a crash of the `sequence` tool could result in a duplicate entry
> being added, so it's best not to rely on uniqueness and instead consider it
> a best-effort anti-spam mitigation.

### Integrating sequenced entries
Although the entries we've added above are now assigned positions in the log, we
still need to update the proof structure state to integrate these new entries.
We use the `integrate` tool for that, again either passing key files or with the 
environment variables set:

```bash
$ go run ./serverless/cmd/integrate --storage_dir=${LOG_DIR} --logtostderr --public_key=key.pub --private_key=key
I0413 17:03:19.239293 4156550 integrate.go:74] Loaded state with roothash
I0413 17:03:19.239468 4156550 integrate.go:113] New log state: size 0x3 hash: 615a21da1739d901be4b1b44aed9cfcfdc044d18842f554a381bba4bff687aff
```

This output says that the integration was successful, and we now have a new log
tree state which contains `0x03` entries, and has the printed log root hash.

Unless further entries are sequenced as above, re-running the `integrate` command
will have no effect:

```bash
$ go run ./serverless/cmd/integrate --storage_dir=${LOG_DIR} --logtostderr --public_key=key.pub --private_key=key
I0413 17:05:10.040900 4156921 integrate.go:74] Loaded state with roothash 615a21da1739d901be4b1b44aed9cfcfdc044d18842f554a381bba4bff687aff
I0413 17:05:10.040976 4156921 integrate.go:94] Nothing to do.
```

### Client

There is a simple client-side tool for querying the log, currently it supports
the following functionality:

#### Inclusion proof verification

We can verify the inclusion of a given leaf in the tree with the `client inclusion`
command:

```bash
$ go run ./serverless/cmd/client/ --logtostderr --public_key=key.pub --log_url=file:///${LOG_DIR}/ inclusion ./CONTRIBUTING.md
I0413 17:09:48.335324 4158369 client.go:99] Leaf "./CONTRIBUTING.md" found at index 0
I0413 17:09:48.335468 4158369 client.go:119] Inclusion verified in tree size 3, with root 0x615a21da1739d901be4b1b44aed9cfcfdc044d18842f554a381bba4bff687aff
```

As expected, requesting an inclusion proof for something not in the log will fail:

```bash
$ go run ./serverless/cmd/client/ --logtostderr --log_url=file:///${LOG_DIR}/ inclusion ./go.mod
F0413 17:13:04.148676 4158991 client.go:72] Command "inclusion" failed: "failed to lookup leaf index: leafhash unknown (open /${LOG_DIR}/leaves/67/48/64/2df7219529a9f2303e8668d60b70a6d7600f22e22fc612c26bd3c399ef: no such file or directory)"
exit status 1
```

> :frog: </br>
> Note that the `--log_url` parameter is a URL, it understands `file://`
> URLs for local filesystem access, but also works with `http[s]://` URLs too - so
> you can directly serve the filesystem contents in `${LOG_DIR}` via HTTP[S] and point
> the client at that server instead and it should work just fine.
>
> E.g.:
>
> ```bash
> $ busybox httpd -f -p 8000 -h ${LOG_DIR}
> ```
> and in another terminal:
>
> ```bash
> $ go run ./serverless/cmd/client/ --logtostderr --log_url=http://localhost:8000 inclusion ./CONTRIBUTING.md
> I0413 17:25:05.799998 4163606 client.go:99] Leaf "./CONTRIBUTING.md" found at index 0
> I0413 17:25:05.801354 4163606 client.go:119] Inclusion verified in tree size 3, with root 0x615a21da1739d901be4b1b44aed9cfcfdc044d18842f554a381bba4bff687aff
> ```

Hosting serverless logs
--------------------------------------

In many cases we'd like to outsource the job of hosting our log to a third
party. There are many possibile ways to do this, one is to use GitHub as both
a public storage provider for serving the log state, and as hosting the process
of updating the log state.

For more details, including example GitHub Action configs, see
[here](./deploy/github).

TODO
----

 - [X] Document structure, design, etc.
 - [X] Integration test.
 - [X] Update client to be able to read tree data from the filesystem via HTTP.
 - [X] Implement & document GitHub actions components.
 - [X] Support for squashing dupes.
 - [ ] Maybe add configs/examples/docs for Cloud Functions, etc. too.
