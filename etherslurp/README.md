# tether

#

Check out this repo (using Go), and cd to it:

```bash
go get -ud github.com/google/trillian-examples/etherslurp
# the warning "package github.com/google/trillian-examples/etherslurp: no Go files in .../src/github.com/google/trillian-examples/etherslurp" is expected
# All terminals need to start in this directory
cd $GOPATH/src/github.com/google/trillian-examples/etherslurp
```

Get a Trillian Log and Map running:

```bash
make trillian

# In the first terminal:
make tlserver

# In another terminal:
make tlsigner

# In another terminal:
make tmserver
```

Create a Log in Trillian:
```bash
# In another terminal
# if you need to recreate the log, first: rm $GOPATH/src/github.com/google/trillian-examples/etherslurp/logid
make createlog
```

Create a Map in Trillian:
```bash
# if you need to recreate the log, first: rm $GOPATH/src/github.com/google/trillian-examples/etherslurp/mapid
make createmap
```

Build and run geth.
We're going to use the rinkeby.io test-net because everything takes too long on
the main net :)

```bash
# to rebuild: rm $GOPATH/src/github.com/ethereum/go-ethereum/build/bin/geth
make geth

# download rinkeby config, and init the data dir:
make initgeth

# Finally, run geth to sync the data:
make rungeth

```

Build and run the tether Follower:

```bash
# Yes, another terminal:
make follower
```

Build and run the tether Mapper:

```bash
# ... yup
make mapper
```

Watch as your diskspace gets eaten.

# UI
If you'd like to inspect the contents of the map, there's a very simple web UI you can use to do so (again, using your saved MAPID):

```bash
# in another terminal, run:
make ui
```

Then surf to locahost:9001
