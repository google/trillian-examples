// This package is the entrypoint for the Firmware Transparency personality server.
// Start the server using:
// go run ./cmd/ft_personality/main.go --logtostderr -v=2
package main

import (
	"flag"
	"net/http"

	"github.com/golang/glog"
	ih "github.com/google/trillian-examples/binary_transparency/firmware/cmd/ft_personality/internal/http"
)

var listenAddr = flag.String("listen", ":8000", "Address:port to listen for requests on")

func main() {
	flag.Parse()
	glog.Infof("Starting FT personality server...")
	srv := &ih.Server{}

	srv.RegisterHandlers()

	glog.Fatal(http.ListenAndServe(*listenAddr, nil))
}
