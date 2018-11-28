// Copyright 2018 Google Inc. All Rights Reserved.
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
	"context"
	"fmt"

	"github.com/google/trillian-examples/gossip/api"

	ct "github.com/google/certificate-transparency-go"
)

// AddCTSTH adds a Certificate Transparency signed tree head to the Hub.
func (c *HubClient) AddCTSTH(ctx context.Context, sourceID string, sth *ct.SignedTreeHead) (*api.SignedGossipTimestamp, error) {
	headData, err := ct.SerializeSTHSignatureInput(*sth)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tree head data: %v", err)
	}
	return c.AddSignedBlob(ctx, sourceID, headData, sth.TreeHeadSignature.Signature)
}
