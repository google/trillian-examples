// This package is the entrypoint for the Firmware Transparency personality server.
// This requires a Trillian instance to be reachable via gRPC and a tree to have
// been provisioned. See the README in the root of this project for instructions.
// Start the server using:
// go run ./cmd/ft_personality/main.go --logtostderr -v=2 --tree_id=$TREE_ID
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/client"
	tt "github.com/google/trillian/types"
	"google.golang.org/grpc"
	ih "github.com/google/trillian-examples/binary_transparency/firmware/cmd/ft_personality/internal/http"

	_ "github.com/google/trillian/merkle/rfc6962" // Load hashers
)

var (
	listenAddr = flag.String("listen", ":8000", "address:port to listen for requests on")

	connectTimeout = flag.Duration("connect_timeout", time.Second, "the timeout for connecting to the backend")
	trillianAddr   = flag.String("trillian", ":8090", "address:port of Trillian Log gRPC service")
	treeID         = flag.Int64("tree_id", -1, "the tree ID of the log to use")
)

func main() {
	flag.Parse()

	if *treeID < 0 {
		glog.Exitf("tree_id is required")
	}
	glog.Infof("Connecting to Trillian Log...")
	tlog, close, err := newTrillianLogger(context.Background(), *connectTimeout, *trillianAddr, *treeID)
	if err != nil {
		glog.Exitf("failed to connect to Trillian: %v", err)
	}
	defer close()

	glog.Infof("Starting FT personality server...")
	srv := ih.NewServer(tlog)

	srv.RegisterHandlers()

	glog.Fatal(http.ListenAndServe(*listenAddr, nil))
}

func newTrillianLogger(ctx context.Context, timeout time.Duration, logAddr string, treeID int64) (*client.LogClient, func(), error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := grpc.DialContext(ctx, logAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, nil, fmt.Errorf("did not connect to trillian on %v: %v", logAddr, err)
	}
	admin := trillian.NewTrillianAdminClient(conn)
	tree, err := admin.GetTree(ctx, &trillian.GetTreeRequest{TreeId: treeID})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get tree %d: %v", treeID, err)
	}
	glog.Infof("Got tree %v", tree)
	v, err := client.NewLogVerifierFromTree(tree)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create verifier from tree: %v", err)
	}

	log := trillian.NewTrillianLogClient(conn)
	c := client.New(treeID, log, v, tt.LogRootV1{})

	return c, func() { conn.Close() }, nil
}
