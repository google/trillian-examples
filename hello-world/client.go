// Copyright 2020 Google LLC
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
    "github.com/google/trillian/merkle"
    "github.com/google/trillian/merkle/rfc6962"

    p "github.com/google/trillian-examples/hello-world/personality"
)

// Personality is the interface to the the Trillian log.
type Personality interface {
    // Append stores an entry in the log.
    Append(context.Context, []byte) p.Chkpt

    // ProveIncl proves the inclusion of a given entry.
    ProveIncl(context.Context, p.Chkpt, []byte) *trillian.Proof

    // UpdateChkpt provides a new checkpoint and proves it is consistent
    // with an old one.
    UpdateChkpt(context.Context, p.Chkpt) (p.Chkpt, *trillian.Proof)
}

// A client is a verifier that maintains a checkpoint as state.
type Client struct {
    v      merkle.LogVerifier
    chkpt  p.Chkpt
    person Personality
}

// NewClient creates a new client with an empty checkpoint and a given
// personality to talk to.
func NewClient(prsn Personality) Client {
    c := Client{person: prsn}
    v := merkle.NewLogVerifier(rfc6962.DefaultHasher)
    c.v = v
    var rootHash []byte
    chkpt := p.Chkpt{LogSize: 0, RootHash: rootHash}
    c.chkpt = chkpt
    return c
}

// VerIncl allows the client to check inclusion of a given entry.
func (c Client) VerIncl(entry []byte, pf *trillian.Proof) bool {
    if pf == nil {
	return false
    }
    leafHash := rfc6962.DefaultHasher.HashLeaf(entry)
    if err := c.v.VerifyInclusionProof(pf.LeafIndex, c.chkpt.LogSize,
	pf.Hashes, c.chkpt.RootHash, leafHash); err != nil {
	return false
    }
    return true
}

// UpdateChkpt allows a client to update its stored checkpoint.
func (c Client) UpdateChkpt(chkptNew p.Chkpt, pf *trillian.Proof) bool {
    // If there is no checkpoint then just use this one no matter what.
    if c.chkpt.LogSize != 0 {
	// Else make sure this new checkpoint is consistent with the current one.
	if pf == nil {
	    return false
	}
	hashes := pf.GetHashes()
	if err := c.v.VerifyConsistencyProof(c.chkpt.LogSize, chkptNew.LogSize,
	    c.chkpt.RootHash, chkptNew.RootHash, hashes); err != nil {
	    return false
	}
    }
    // If all is good then set this as the new checkpoint.
    c.chkpt = chkptNew
    return true
}
