// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Runs a simple Trillian personality.
package personality

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian/merkle/rfc6962/hasher"
	tt "github.com/google/trillian/types"
	"google.golang.org/grpc"
)

var (
	connectTimeout = flag.Duration("connect_timeout", 5*time.Second, "the timeout for connecting to the backend")
)

type TrillianP struct {
	l      trillian.TrillianLogClient
	treeID int64
}

// NewPersonality creates a new Trillian personality from the flags.
func NewPersonality(logAddr string, treeID int64) (*TrillianP, error) {
	if treeID <= 0 {
		return nil, fmt.Errorf("tree_id must be provided and positive, got %d", treeID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *connectTimeout)
	defer cancel()
	conn, err := grpc.DialContext(ctx, logAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("did not connect to trillian on %v: %v", logAddr, err)
	}

	log := trillian.NewTrillianLogClient(conn)

	return &TrillianP{
		l:      log,
		treeID: treeID,
	}, nil
}

// formLeaf creates a Trillian log leaf from an entry.
func (p *TrillianP) formLeaf(entry []byte) *trillian.LogLeaf {
	leafHash := hasher.DefaultHasher.HashLeaf(entry)
	return &trillian.LogLeaf{
		LeafValue:      entry,
		MerkleLeafHash: leafHash,
	}
}

// GetChkpt gets the latest checkpoint.
func (p *TrillianP) GetChkpt(ctx context.Context) (*log.Checkpoint, error) {
	req := trillian.GetLatestSignedLogRootRequest{LogId: p.treeID}
	resp, err := p.l.GetLatestSignedLogRoot(ctx, &req)
	if err != nil {
		return nil, err
	}
	// Unpack the response and convert it to the local Checkpoint
	// representation.
	root := resp.GetSignedLogRoot()
	var logRoot tt.LogRootV1
	if err := logRoot.UnmarshalBinary(root.LogRoot); err != nil {
		return nil, err
	}
	return &log.Checkpoint{
		Ecosystem: "Hello World v0",
		Hash:      logRoot.RootHash,
		Size:      logRoot.TreeSize,
	}, nil
}

// Append adds an entry to the Trillian log and waits to return the new checkpoint.
func (p *TrillianP) Append(ctx context.Context, entry []byte) (*log.Checkpoint, error) {
	// First get the latest checkpoint.
	chkpt, _ := p.GetChkpt(ctx)
	leaf := p.formLeaf(entry)
	req := trillian.QueueLeafRequest{LogId: p.treeID, Leaf: leaf}
	if _, err := p.l.QueueLeaf(ctx, &req); err != nil {
		return nil, err
	}
	// Now fetch the new checkpoint, keep going until it's there and
	// return an error at some point if it isn't.
	for start := time.Now(); time.Since(start) < 5*time.Second; {
		chkptNew, err := p.GetChkpt(ctx)
		if err != nil {
			return nil, err
		}
		if chkpt.Size < chkptNew.Size {
			return chkptNew, nil
		}
	}
	return nil, fmt.Errorf("did not get an updated checkpoint")
}

// ProveIncl returns an inclusion proof for a given checkpoint and entry.
func (p *TrillianP) ProveIncl(ctx context.Context, chkpt *log.Checkpoint, entry []byte) (*trillian.Proof, error) {
	// Form the leaf from the entry.
	leaf := p.formLeaf(entry)
	// Form the request according to the Trillian API.
	req := trillian.GetInclusionProofByHashRequest{
		LogId:    p.treeID,
		LeafHash: leaf.MerkleLeafHash,
		TreeSize: int64(chkpt.Size),
	}
	// Process the response.
	resp, err := p.l.GetInclusionProofByHash(ctx, &req)
	if err != nil {
		return nil, err
	}
	return resp.GetProof()[0], nil
}

// UpdateChkpt gets the latest checkpoint for the Trillian log and proves its
// consistency with a provided one.
func (p *TrillianP) UpdateChkpt(ctx context.Context, chkpt *log.Checkpoint) (*log.Checkpoint, *trillian.Proof, error) {
	// First get the latest checkpoint.
	chkptNew, err := p.GetChkpt(ctx)
	if err != nil {
		return nil, nil, err
	}
	// Now get a consistency proof if one is needed.
	var pf *trillian.Proof
	if chkptNew.Size > chkpt.Size {
		req := trillian.GetConsistencyProofRequest{
			LogId:          p.treeID,
			FirstTreeSize:  int64(chkpt.Size),
			SecondTreeSize: int64(chkptNew.Size),
		}
		resp, err := p.l.GetConsistencyProof(ctx, &req)
		if err != nil {
			return nil, nil, err
		}
		pf = resp.GetProof()
	}
	return chkptNew, pf, nil
}
