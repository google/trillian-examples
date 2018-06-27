This tutorial will show you how to get started with Trillian, build a
quick working demo, and then transform that into a verifiable,
reliable service.

Step 1
------

We need a source of data that we are going to log. This could be
anything, but for this tutorial, I've chosen the [GDS Registers
project](https://www.gov.uk/government/publications/registers/registers).

In this step, we start with a simple program that reads a register and
dumps its contents to stdout.

You can run it with

```go run dump/main.go```

By default it uses the ["register" register (a list of all
registers)](https://register.register.gov.uk/) but you can point it at
others with the `--register` flag.

Step 2
------

Next, we are going to put the data into a Trillian log. The first step
is run the log server and create a log.

Build Trillian:

```make trillian```

(Note that if you have already have Trillian source, this will update
it, but the update does not always seem to be reliable, see the
[Trillian README](https://github.com/google/trillian#using-the-code))

Run the log server (this will tie up a terminal window):

```make tlserver```

And create a log (if you need to redo this, you can run `make
deletelog` first):

```make createlog```

So now we have a log ready for entries from the register. To fill that
log, the application connects to the log using `grpc.Dial()`, then
creates a new log client with `trillian.NewTrillianLogClient`.

It then feeds each entry as we get it to the log, using
`QueueLeaf()` on the log client. The rest is just housekeeping.

I chose to use JSON to encode the leaves because the registers
themselves use JSON. JSON is not actually a very good encoding format,
it is too limited, so I would normally advise something else, such as
protobufs.

You can run the logger with:

```
go run dump/main.go -log_id=`cat logid`
```

Note that if you run it a second time, it will not usually add any new
entries, though this is not guaranteed by Trillian, depending on
exactly how it is configured. That is, it is possible to get duplicate
log entries under some circumstances.

Step 3
------

Now we've put stuff into the log, how do we get it out again? If all
we want to do is dump the records, then it's easy. We'll get fancy
later.

First of all, nothing gets actually committed to the log unless we're
running a signer. So let's get one started - this will tie another
terminal up.

```make tlsigner```

Then our application can connect to the log in the usual way and
retrieve entries from the log using `GetLeavesByRange` on the log
client. We first find out how many entries there are in the log with
`GetLatestSignedLogRoot` (we'll come back to why it's signed later).

You can run it like this:

```
go run extract/main.go --log_id=`cat logid`
```

One subtlety to pay attention to is server skew - in a real system,
it is entirely possible that not all Trillian servers will be exactly
in sync, which could lead to us requesting leaves that the server
doesn't have. In that case, the server will return the number of
leaves it does have so we can take appropriate action. Although this
can't happen in this test setup (since there's only one server) it is
a bad idea to ignore the problem, so we deal with it now.
