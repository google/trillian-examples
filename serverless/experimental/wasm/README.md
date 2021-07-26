# Serverless WASM

This directory contains a version of the `serverless` tooling cross-compiled to WebAssembly (WASM).

This is an entirely experimental thing, mostly intended to explore the possibility
of creating a semi "live" demo/toy to explore the concepts of transparent logging
and the related ecosystem functions from within a browser environment.

## Instructions

To build and run up a webserver which serves the WASM+support files, run the following commands from the root of this repo:

```bash
$ GOOS=js GOARCH=wasm go build -o ./serverless/experimental/wasm/main.wasm -tags wasm ./serverless/experimental/wasm/
```

then copy over the Go WASM shim into this directory:

```bash
$ cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" serverless/experimental/wasm/
```

Run up a webserver to serve the files (use something which can infer the correct ContentType header to send). You can use `goexec` for this (install `goexec` with `go get -u github.com/shurcooL/goexec`):

```bash
$ goexec "http.ListenAndServe(`:8080`, http.FileServer(http.Dir(`serverless/experimental/wasm`)))"
```

Finally, point your browser at http://localhost:8080