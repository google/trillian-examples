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
