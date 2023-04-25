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

// Package verify supports verification that the downloaded contents match
// the root hash commitment made in a log checkpoint.
package verify

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/clone/logdb"
	"github.com/transparency-dev/merkle/compact"
)

// LeafHashFn returns the leaf hash (commitment) to the given preimage at the given log index.
type LeafHashFn func(index uint64, data []byte) []byte

// NewLogVerifier returns a LogVerifier which obtains raw leaves from `db`, and uses the given
// hash functions to construct a merkle tree.
func NewLogVerifier(db *logdb.Database, lh LeafHashFn, ih compact.HashFn) LogVerifier {
	return LogVerifier{
		db: db,
		lh: lh,
		rf: &compact.RangeFactory{Hash: ih},
	}
}

// LogVerifier calculates the Merkle root of a log.
type LogVerifier struct {
	db *logdb.Database
	lh LeafHashFn
	rf *compact.RangeFactory
}

// MerkleRoot calculates the Merkle root hash of its log at the given size.
func (v LogVerifier) MerkleRoot(ctx context.Context, size uint64) ([]byte, [][]byte, error) {
	cr := v.rf.NewEmptyRange(0)
	from := uint64(0)

	if cpSize, _, crBs, err := v.db.GetLatestCheckpoint(ctx); err != nil {
		if err != logdb.ErrNoDataFound {
			return nil, nil, err
		}
	} else if size >= cpSize {
		from = cpSize
		cr, err = v.rf.NewRange(0, cpSize, crBs)
		if err != nil {
			return nil, nil, err
		}
	}

	leaves := make(chan []byte, 1)
	errc := make(chan error)
	glog.V(1).Infof("Streaming leaves [%d, %d)", from, size)
	go v.db.StreamLeaves(from, size, leaves, errc)

	index := from
	for leaf := range leaves {
		select {
		case err := <-errc:
			return nil, nil, fmt.Errorf("failed to get leaves from DB: %w", err)
		default:
		}
		if err := cr.Append(v.lh(index, leaf), nil); err != nil {
			glog.Errorf("cr.Append(): %v", err)
		}
		index++
	}
	if index != size {
		return nil, nil, fmt.Errorf("expected to receive %d leaves but got %d", size, index)
	}
	root, err := cr.GetRootHash(nil)
	return root, cr.Hashes(), err
}
