// Copyright 2021 Google LLC. All Rights Reserved.
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
	"errors"
	"net/url"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
)

type MapClient struct {
	URL *url.URL
}

func NewMapClient(mapURL string) (*MapClient, error) {
	u, err := url.Parse(mapURL)
	if err != nil {
		return nil, err
	}
	return &MapClient{
		URL: u,
	}, nil
}

// MapCheckpoint returns the Checkpoint for the latest map revision.
// This map root needs to be taken on trust that it isn't forked etc.
// To remove this trust, the map roots should be stored in a log, and
// this would further return:
// * A Log Checkpoint for the MapCheckpointLog
// * An inclusion proof for this checkpoint within it
func (c *MapClient) MapCheckpoint() (api.MapCheckpoint, error) {
	// TODO(mhutchinson): http://mapserver/latest
	return api.MapCheckpoint{}, errors.New("unimplemented")
}

// Aggregation returns the value committed to by the map under the given key,
// with an inclusion proof.
func (c *MapClient) Aggregation(mcp api.MapCheckpoint, fwIndex uint64) (api.AggregatedFirmware, api.MapInclusionProof, error) {
	// TODO(mhutchinson): Fill out according to psuedocode below

	// key := fmt.Sprintf("summary:%d", fwIndex)
	// kbs := sha512.Sum512_256([]byte(key))
	// tiles := fetch(http://mapserver/tiles/$kbs)
	// leafhash := get the value hash at key from `tiles`
	// Simultaneously:
	//   * compute inclusion proof locally from tiles
	//   * value := fetch(http://mapserver/value/$leafhash)
	// Confirm the value returned matches the leafhash, return it all
	return api.AggregatedFirmware{}, api.MapInclusionProof{}, errors.New("unimplemented")
}
