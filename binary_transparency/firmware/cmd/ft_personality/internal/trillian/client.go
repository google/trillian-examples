// Package trillian represents the log for the needs of this personality.
package trillian

import (
	"context"
	"fmt"
	"sync"
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
	*client.LogVerifier

	logID      int64
	client     trillian.TrillianLogClient
	golden     types.LogRootV1
	goldenLock sync.Mutex
	updateLock sync.Mutex
	done       func()
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

	// This implicitly trusts whatever state the log now reports.
	// If the log is considered outside of the personality's TCB
	// (https://en.wikipedia.org/wiki/Trusted_computing_base) then this
	// initial state should be read from some local storage from the last
	// time the personality ran.
	golden := tt.LogRootV1{}

	client := &Client{
		LogVerifier: v,
		logID:       treeID,
		client:      log,
		golden:      golden,
		done:        func() { conn.Close() },
	}

	return client, nil
}

// AddFirmwareManifest adds the firmware manifest to the log if it isn't already present.
func (c *Client) AddFirmwareManifest(ctx context.Context, data []byte) error {
	leafHash := c.Hasher.HashLeaf(data)
	leaf := &trillian.LogLeaf{
		LeafValue:      data,
		MerkleLeafHash: leafHash,
	}

	_, err := c.client.QueueLeaf(ctx, &trillian.QueueLeafRequest{
		LogId: c.logID,
		Leaf:  leaf,
	})
	return err
}

// GetRoot returns the most recent root seen by this client.
// Use UpdateRoot() to update this client's view of the latest root.
func (c *Client) GetRoot() *types.LogRootV1 {
	c.goldenLock.Lock()
	defer c.goldenLock.Unlock()

	// Copy the internal trusted root in order to prevent clients from modifying it.
	ret := c.golden
	return &ret
}

// UpdateRoot retrieves the current SignedLogRoot, verifying it against roots this client has
// seen in the past, and updating the currently trusted root if the new root verifies, and is
// newer than the currently trusted root.
// After returning, the most recent verified root will be obtainable via c.GetRoot().
func (c *Client) UpdateRoot(ctx context.Context) error {
	// Only one root update should be running at any point in time, because
	// the update involves a consistency proof from the old value, and if the
	// old value could change along the way (in another goroutine) then the
	// result could be inconsistent.
	c.updateLock.Lock()
	defer c.updateLock.Unlock()
	golden := c.GetRoot()

	resp, err := c.client.GetLatestSignedLogRoot(ctx,
		&trillian.GetLatestSignedLogRootRequest{
			LogId:         c.logID,
			FirstTreeSize: int64(golden.TreeSize),
		})
	if err != nil {
		return err
	}
	var newRoot types.LogRootV1
	if err := newRoot.UnmarshalBinary(resp.GetSignedLogRoot().LogRoot); err != nil {
		return err
	}

	if newRoot.TreeSize <= golden.TreeSize {
		return nil
	}

	// The new root is fresher than our golden, let's check consistency and update
	// the golden root if it verifies.
	if _, err := c.VerifyRoot(golden, resp.GetSignedLogRoot(), resp.GetProof().GetHashes()); err != nil {
		return err
	}

	c.goldenLock.Lock()
	defer c.goldenLock.Unlock()

	c.golden = newRoot
	return nil
}

// Close finishes the underlying connections and tidies up after the Client is finished.
func (c *Client) Close() {
	c.done()
}
