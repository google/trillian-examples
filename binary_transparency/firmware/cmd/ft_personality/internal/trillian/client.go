// Copyright 2020 Google LLC. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package trillian represents the log for the needs of this personality.
package trillian

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/trillian-examples/binary_transparency/firmware/cmd/ft_personality/internal/trees"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/client"
	tt "github.com/google/trillian/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client represents the personality's view of the Trillian Log.
type Client struct {
	*client.LogVerifier

	logID      int64
	client     trillian.TrillianLogClient
	golden     tt.LogRootV1
	goldenLock sync.Mutex
	updateLock sync.Mutex
	done       func()
}

// NewClient returns a new client that will read/write to its tree using
// the Trillian gRPC API URL provided, with the given timeout for connections.
// The Client returned should have Close called by the owner when done.
func NewClient(ctx context.Context, timeout time.Duration, logAddr string, treeStorage *trees.TreeStorage) (*Client, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := grpc.DialContext(ctx, logAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("did not connect to trillian on %v: %v", logAddr, err)
	}

	tree, err := treeStorage.EnsureTree(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("failed to get/create tree: %v", err)
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
		logID:       tree.TreeId,
		client:      log,
		golden:      golden,
		done:        func() { conn.Close() },
	}

	return client, nil
}

// AddSignedStatement adds the statement to the log if it isn't already present.
func (c *Client) AddSignedStatement(ctx context.Context, data []byte) error {
	leafHash := c.BuildLeaf(data).MerkleLeafHash
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

// Root returns the most recent root seen by this client.
// Use UpdateRoot() to update this client's view of the latest root.
func (c *Client) Root() *tt.LogRootV1 {
	c.goldenLock.Lock()
	defer c.goldenLock.Unlock()

	// Copy the internal trusted root in order to prevent clients from modifying it.
	ret := c.golden
	return &ret
}

// UpdateRoot retrieves the current SignedLogRoot, verifying it against roots this client has
// seen in the past, and updating the currently trusted root if the new root verifies, and is
// newer than the currently trusted root.
// After returning, the most recent verified root will be obtainable via c.Root().
func (c *Client) UpdateRoot(ctx context.Context) error {
	// Only one root update should be running at any point in time, because
	// the update involves a consistency proof from the old value, and if the
	// old value could change along the way (in another goroutine) then the
	// result could be inconsistent.
	c.updateLock.Lock()
	defer c.updateLock.Unlock()
	golden := c.Root()

	resp, err := c.client.GetLatestSignedLogRoot(ctx,
		&trillian.GetLatestSignedLogRootRequest{
			LogId:         c.logID,
			FirstTreeSize: int64(golden.TreeSize),
		})
	if err != nil {
		return err
	}
	var newRoot tt.LogRootV1
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

// ConsistencyProof gets the consistency proof between two given tree sizes.
func (c *Client) ConsistencyProof(ctx context.Context, from, to uint64) ([][]byte, error) {
	cp, err := c.client.GetConsistencyProof(ctx, &trillian.GetConsistencyProofRequest{
		LogId:          c.logID,
		FirstTreeSize:  int64(from),
		SecondTreeSize: int64(to),
	})
	if err != nil {
		return nil, err
	}
	return cp.Proof.Hashes, nil
}

// FirmwareManifestAtIndex gets the value at the given index and an inclusion proof
// to the given tree size.
func (c *Client) FirmwareManifestAtIndex(ctx context.Context, index, treeSize uint64) ([]byte, [][]byte, error) {
	ip, err := c.client.GetEntryAndProof(ctx, &trillian.GetEntryAndProofRequest{
		LogId:     c.logID,
		LeafIndex: int64(index),
		TreeSize:  int64(treeSize),
	})
	if err != nil {
		return nil, nil, err
	}
	return ip.Leaf.LeafValue, ip.Proof.Hashes, nil
}

// InclusionProofByHash gets an inclusion proof in the specified tree size for the
// first leaf found with the specified hash.
func (c *Client) InclusionProofByHash(ctx context.Context, hash []byte, treeSize uint64) (uint64, [][]byte, error) {
	ip, err := c.client.GetInclusionProofByHash(ctx, &trillian.GetInclusionProofByHashRequest{
		LogId:    c.logID,
		LeafHash: hash,
		TreeSize: int64(treeSize),
	})
	if err != nil {
		return 0, nil, err
	}
	if len(ip.Proof) == 0 {
		return 0, nil, fmt.Errorf("no leaves found for hash 0x%x", hash)
	}
	return uint64(ip.Proof[0].LeafIndex), ip.Proof[0].Hashes, nil
}

// Close finishes the underlying connections and tidies up after the Client is finished.
func (c *Client) Close() {
	c.done()
}
