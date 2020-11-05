// Package trillian represents the log for the needs of this personality.
package trillian

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/client"
	"github.com/google/trillian/types"
	tt "github.com/google/trillian/types"
	"google.golang.org/grpc"
)

// Client represents the personality's view of the Trillian Log.
type Client struct {
	c    *client.LogClient
	done func()
}

// NewClient returns a new client that will read/write to the given treeID at
// the Trillian gRPC API URL provided, with the given timeout for connections.
// The Client returned should have Close called by the owner when done.
func NewClient(ctx context.Context, timeout time.Duration, logAddr string, treeID int64) (*Client, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := grpc.DialContext(ctx, logAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("did not connect to trillian on %v: %v", logAddr, err)
	}

	// N.B. Using the admin interface from the personality is not good practice for
	// a production system. This simply allows a convenient way of getting the tree
	// for the sake of getting the FT demo up and running.
	admin := trillian.NewTrillianAdminClient(conn)
	tree, err := admin.GetTree(ctx, &trillian.GetTreeRequest{TreeId: treeID})
	if err != nil {
		return nil, fmt.Errorf("failed to get tree %d: %v", treeID, err)
	}
	glog.Infof("Got tree %v", tree)

	v, err := client.NewLogVerifierFromTree(tree)
	if err != nil {
		return nil, fmt.Errorf("failed to create verifier from tree: %v", err)
	}

	log := trillian.NewTrillianLogClient(conn)
	c := client.New(treeID, log, v, tt.LogRootV1{})

	// Start off with an up-to-date root
	c.UpdateRoot(ctx)
	return &Client{
		c:    c,
		done: func() { conn.Close() },
	}, nil
}

// AddFirmwareManifest adds the firmware manifest to the log if it isn't already present.
// TODO(mhutchinson): This blocks until the statement is integrated into the log.
// This _may_ be OK for a demo, but it really should be split.
func (c *Client) AddFirmwareManifest(ctx context.Context, data []byte) error {
	return c.c.AddLeaf(ctx, data)
}

// GetRoot returns the most recent root seen by this client.
// Due to the nature of distributed systems there can be no guarantee that this is
// globally the freshest root.
// TODO(mhutchinson): This should have some promise of freshness, which could be obtained
// by periodically updating the root in the background if we haven't fetched it for other
// reasons.
func (c *Client) GetRoot() *types.LogRootV1 {
	return c.c.GetRoot()
}

// Close finishes the underlying connections and tidies up after the Client is finished.
func (c *Client) Close() {
	c.done()
}
