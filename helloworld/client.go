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

// Runs a simple client, designed to interact with a personality.
package helloworld

import (
	"context"

	trillian "github.com/google/trillian"
	"github.com/google/trillian/merkle/logverifier"
	"github.com/google/trillian/merkle/rfc6962/hasher"

	"github.com/google/trillian-examples/formats/log"
)

// Personality is the front-end for the Trillian log.
type Personality interface {
	// Append stores an entry in the log and, once it is there, returns a
	// new checkpoint that commits to the new entry (in addition to all
	// previous ones).
	Append(context.Context, []byte) (*log.Checkpoint, error)

	// ProveIncl proves the inclusion of a given entry.  It is the job of
	// the client to verify this inclusion proof in order to convince
	// itself that the entry really has been included in the log.
	ProveIncl(context.Context, *log.Checkpoint, []byte) (*trillian.Proof, error)

	// UpdateChkpt provides a new checkpoint and proves it is consistent
	// with an old one.  Again, it is the job of the client to verify this
	// consistency proof.
	UpdateChkpt(context.Context, *log.Checkpoint) (*log.Checkpoint, *trillian.Proof, error)
}

// A client is a verifier that maintains a checkpoint as state.
type Client struct {
	v      logverifier.LogVerifier
	chkpt  *log.Checkpoint
	person Personality
}

// NewClient creates a new client with an empty checkpoint and a given
// personality to talk to.  In real usage, a client should persist this
// checkpoint across different runs to ensure consistency.
func NewClient(prsn Personality) Client {
	v := logverifier.New(hasher.DefaultHasher)
	var rootHash []byte
	chkpt := &log.Checkpoint{
		Ecosystem: "Hello world v0",
		Size:      0,
		Hash:      rootHash,
	}
	return Client{
		person: prsn,
		v:      v,
		chkpt:  chkpt,
	}
}

// VerIncl allows the client to check inclusion of a given entry.
func (c Client) VerIncl(entry []byte, pf *trillian.Proof) bool {
	leafHash := hasher.DefaultHasher.HashLeaf(entry)
	if err := c.v.VerifyInclusionProof(pf.LeafIndex, int64(c.chkpt.Size),
		pf.Hashes, c.chkpt.Hash, leafHash); err != nil {
		return false
	}
	return true
}

// UpdateChkpt allows a client to update its stored checkpoint.  In a real use
// case it would be important for the client to check the signature contained
// in the checkpoint before verifying consistency.
func (c Client) UpdateChkpt(chkptNew *log.Checkpoint, pf *trillian.Proof) bool {
	// If there is no checkpoint then just use this one no matter what.
	if c.chkpt.Size != 0 {
		// Else make sure this new checkpoint is consistent with the current one.
		hashes := pf.GetHashes()
		if err := c.v.VerifyConsistencyProof(int64(c.chkpt.Size), int64(chkptNew.Size),
			c.chkpt.Hash, chkptNew.Hash, hashes); err != nil {
			return false
		}
	}
	// If all is good then set this as the new checkpoint.
	c.chkpt = chkptNew
	return true
}
