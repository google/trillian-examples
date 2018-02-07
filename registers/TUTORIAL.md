Step 1
------

We need a source of data that we are going to log. This could be
anything, but for this tutorial, I've chosen the [GDS Registers
project](https://www.gov.uk/government/publications/registers/registers).

In this step, we start with a simple program that reads a register and
dumps its contents to stdout.

You can run it with

```go run dump/main.go```

By default it uses the "register" register (a list of all registers)
but you can point it at others with the ```--register``` flag.

Step 2
------

Next, we are going to put the data into a Trillian log. The first step
is run the log server and create a log.

Build Trillian:

```make trillian```

Run the log server (this will tie up a terminal window):

```make tlserver```

And create a log (if you need to redo this, you'll need to delete
logid):

```make createlog```

So now we have a log ready for entries from the register. To fill that
log, we connect to the log using ```grpc.Dial()```, then create a new
log client with ```trillian.NewTrillianLogClient```.

We then feed each entry as we get it to the log, using
```QueueLeaf()``` on the log client. The rest is just housekeeping.

I chose to use JSON to encode the leaves because the registers
themselves use JSON. JSON is not actually a very good encoding format,
it is too limited, so I would normally advise something else, such as
protobufs.

You can run the logger with:

```
go run dump/main.go -log_id=`cat logid`
```

Note that if you run it a second time, it will not add any new entries
(unless the underlying GDS register has changed) because duplicate
entries are ignored by Trillian, by default at least. (FIXME: can
dupes occur if added fast enough? Presumably yes?)
