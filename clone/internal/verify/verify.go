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
package verify

import (
	"fmt"

	"github.com/google/trillian/merkle/compact"
)

// StreamLeaves is a function that returns all the leaves in [start, end), in order, on the `out` channel.
// Errors are returned via `errc`, and `out` will be closed when all data has been returned.
type StreamLeaves func(start, end uint64, out chan []byte, errc chan error)

// LeafHashFn returns the leaf hash (commitment) to the given preimage at the given log index.
type LeafHashFn func(index uint64, data []byte) []byte

// NewLogVerifier returns a LogVerifier which obtains raw leaves from `streamLeaves`, and uses the given
// hash functions to construct a merkle tree.
func NewLogVerifier(streamLeaves StreamLeaves, lh LeafHashFn, ih compact.HashFn) LogVerifier {
	return LogVerifier{
		streamLeaves: streamLeaves,
		lh:           lh,
		rf:           &compact.RangeFactory{Hash: ih},
	}
}

// LogVerifier calculates the Merkle root of a log.
type LogVerifier struct {
	streamLeaves StreamLeaves
	lh           LeafHashFn
	rf           *compact.RangeFactory
}

// MerkleRoot calculates the Merkle root hash of its log at the given size.
func (v LogVerifier) MerkleRoot(size uint64) ([]byte, error) {
	r := v.rf.NewEmptyRange(0)

	leaves := make(chan []byte, 1)
	errc := make(chan error)

	go v.streamLeaves(0, size, leaves, errc)

	index := uint64(0)
	for leaf := range leaves {
		select {
		case err := <-errc:
			return nil, fmt.Errorf("failed to get leaves from DB: %w", err)
		default:
		}
		r.Append(v.lh(index, leaf), nil)
		index++
	}
	if index != size {
		return nil, fmt.Errorf("expected to receive %d leaves but got %d", size, index)
	}
	return r.GetRootHash(nil)
}
