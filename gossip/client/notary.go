// Copyright 2019 Google Inc. All Rights Reserved.
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

package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/gossip/api"
)

// Code is adapted from golang.org/x/exp/sumdb/internal/note
// and copied here to avoid a dependency on an experimental repo.
// TODO(daviddrysdale): use official repo once stable
var (
	sigSplit  = []byte("\n\n")
	sigPrefix = []byte("— ") // Watch out: this is em-dash (U+2014) not minus.
)

// AddSignedNote adds a Go notary signed note to the Hub. The signer name in the
// note must match the sourceID provided.
func (c *HubClient) AddSignedNote(ctx context.Context, sourceID string, note []byte) (*api.SignedGossipTimestamp, error) {
	// Must end with signature block preceded by blank line.
	split := bytes.LastIndex(note, sigSplit)
	if split < 0 {
		return nil, errors.New("failed to parse signed note, no separator")
	}
	// Signatures are over the message block (including the first of the two newlines).
	data, sigs := note[:split+1], note[split+2:]
	if len(sigs) == 0 {
		return nil, errors.New("signed note has no signatures")
	}
	for i, sig := range strings.Split(string(sigs), "\n") {
		// Signature lines are of form:
		//   "<prefix> <source_name> <b64data>\n"
		// like:
		//   "— notary.golang.org Az3grgT1zlVmN6eQlpjuauyUoba9jNSZUIKp3OdvMbTJ/kDuSr56nSS504OtjPmteNzMAQWvfjzI82pD/SEgiqJigQ0=\n"
		if !bytes.HasPrefix([]byte(sig), sigPrefix) {
			glog.Warningf("skipping signature %d with missing prefix", i)
			continue
		}
		line := sig[len(sigPrefix):]
		spaceIdx := strings.Index(line, " ")
		if spaceIdx == -1 {
			glog.Warningf("skipping signature %d with missing name/sig separator", i)
			continue
		}
		name := line[:spaceIdx]
		b64 := line[spaceIdx+1:]
		if name != sourceID {
			// This isn't from the source we're interested in (but there may be more signatures).
			continue
		}

		if len(b64) == 0 {
			return nil, fmt.Errorf("empty signature [%d]", i)
		}
		sig, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, fmt.Errorf("signature [%d] failed to parse: %v", i, err)
		}
		if len(sig) < 5 {
			return nil, fmt.Errorf("signature [%d] too short: %d", i, len(sig))
		}
		// The base64-encoded data is 4 bytes of key hash then the signature.
		return c.AddSignedBlob(ctx, sourceID, data, sig[4:])
	}
	return nil, fmt.Errorf("no signature for source %s found", sourceID)
}
