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
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian/merkle/coniks"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/merkle/smt"
	"github.com/google/trillian/storage/tree"
	"golang.org/x/sync/errgroup"
)

type MapClient struct {
	mapURL *url.URL
}

func NewMapClient(mapURL string) (*MapClient, error) {
	u, err := url.Parse(mapURL)
	if err != nil {
		return nil, err
	}
	return &MapClient{
		mapURL: u,
	}, nil
}

// MapCheckpoint returns the Checkpoint for the latest map revision.
// This map root needs to be taken on trust that it isn't forked etc.
// To remove this trust, the map roots should be stored in a log, and
// this would further return:
// * A Log Checkpoint for the MapCheckpointLog
// * An inclusion proof for this checkpoint within it
func (c *MapClient) MapCheckpoint() (api.MapCheckpoint, error) {
	mcp := api.MapCheckpoint{}
	u, err := c.mapURL.Parse(api.MapHTTPGetCheckpoint)
	if err != nil {
		return mcp, err
	}
	r, err := http.Get(u.String())
	if err != nil {
		return mcp, err
	}
	if r.StatusCode != http.StatusOK {
		return mcp, errFromResponse("failed to fetch checkpoint", r)
	}

	if err := json.NewDecoder(r.Body).Decode(&mcp); err != nil {
		return mcp, err
	}
	// TODO(mhutchinson): Check signature
	return mcp, nil
}

// Aggregation returns the value committed to by the map under the given key,
// with an inclusion proof.
func (c *MapClient) Aggregation(ctx context.Context, rev uint64, fwIndex uint64) (api.AggregatedFirmware, api.MapInclusionProof, error) {
	start := time.Now()

	errs, _ := errgroup.WithContext(ctx)
	// Simultaneously fetch all tiles:
	tiles := make([]api.MapTile, api.MapPrefixStrata+1)
	kbs := sha512.Sum512_256([]byte(fmt.Sprintf("summary:%d", fwIndex)))
	for i := range tiles {
		i := i
		errs.Go(func() error {
			path := kbs[:i]
			var t api.MapTile
			err := c.fetch(fmt.Sprintf("%s/in-revision/%d/at-path/%s", api.MapHTTPGetTile, rev, base64.URLEncoding.EncodeToString(path)), &t)
			if err != nil {
				return err
			}
			tiles[i] = t
			return nil
		})
	}

	var agg api.AggregatedFirmware
	errs.Go(func() error {
		return c.fetch(fmt.Sprintf("%s/in-revision/%d/for-firmware-at-index/%d", api.MapHTTPGetAggregation, rev, fwIndex), &agg)
	})

	if err := errs.Wait(); err != nil {
		return api.AggregatedFirmware{}, api.MapInclusionProof{}, err
	}

	ipt := newInclusionProofTree(api.MapTreeID, coniks.Default, kbs[:])
	for i := api.MapPrefixStrata; i >= 0; i-- {
		tile := tiles[i]
		nodes := make([]smt.Node, len(tile.Leaves))
		for j, l := range tile.Leaves {
			nodes[j] = toNode(tile.Path, l)
		}
		hs, err := smt.NewHStar3(nodes, ipt.hasher.HashChildren,
			uint(len(tile.Path)+len(tile.Leaves[0].Path))*8, uint(len(tile.Path))*8)
		if err != nil {
			return agg, api.MapInclusionProof{}, fmt.Errorf("failed to create HStar3 for tile %x: %v", tile.Path, err)
		}
		res, err := hs.Update(ipt)
		if err != nil {
			return agg, api.MapInclusionProof{}, fmt.Errorf("failed to hash tile %x: %v", tile.Path, err)
		} else if got, want := len(res), 1; got != want {
			return agg, api.MapInclusionProof{}, fmt.Errorf("wrong number of roots for tile %x: got %v, want %v", tile.Path, got, want)
		}
	}

	glog.V(1).Infof("Got aggregation (%s): %+v", time.Since(start), agg)
	return agg, *ipt.proof, nil
}

// fetch gets the JSON object from the given path and decodes it into the result type.
// This may need to be extended to return the raw bytes too for operations that need to
// hash the raw bytes.
func (c *MapClient) fetch(path string, result interface{}) error {
	u, err := c.mapURL.Parse(path)
	if err != nil {
		return err
	}
	r, err := http.Get(u.String())
	if err != nil {
		return err
	}
	if r.StatusCode != http.StatusOK {
		return errFromResponse(fmt.Sprintf("failed to fetch %s", path), r)
	}
	body := r.Body
	defer body.Close()
	if err := json.NewDecoder(body).Decode(result); err != nil {
		return err
	}
	return nil
}

// toNode converts a MapTileLeaf into the equivalent Node for HStar3.
func toNode(prefix []byte, l api.MapTileLeaf) smt.Node {
	path := make([]byte, 0, len(prefix)+len(l.Path))
	path = append(append(path, prefix...), l.Path...)
	return smt.Node{
		ID:   tree.NewNodeID2(string(path), uint(len(path))*8),
		Hash: l.Hash,
	}
}

// inclusionProofTree is a NodeAccessor for an empty tree with the given ID.
// As values are set on the tree, an inclusion proof is generated containing
// the siblings computed.
type inclusionProofTree struct {
	treeID int64
	hasher hashers.MapHasher
	target tree.NodeID2
	proof  *api.MapInclusionProof
}

func newInclusionProofTree(treeID int64, hasher hashers.MapHasher, target []byte) inclusionProofTree {
	return inclusionProofTree{
		treeID: treeID,
		hasher: hasher,
		target: tree.NewNodeID2(string(target), 256),
		proof: &api.MapInclusionProof{
			Key:   target,
			Proof: make([][]byte, 256),
		},
	}
}

func (e inclusionProofTree) Get(id tree.NodeID2) ([]byte, error) {
	return e.hasher.HashEmpty(e.treeID, id), nil
}

func (e inclusionProofTree) Set(id tree.NodeID2, hash []byte) {
	if id == e.target {
		e.proof.Value = hash
		glog.V(2).Infof("inclusionProofTree: set value for target: %x", hash)
		return
	}
	stem := e.target.Prefix(id.BitLen())
	if stem == id.Sibling() {
		e.proof.Proof[id.BitLen()-1] = hash
		glog.V(2).Infof("inclusionProofTree: set sibling at depth %d: %x", id.BitLen(), hash)
	}
}
