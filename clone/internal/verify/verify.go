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

// FetchLeaves returns `count` leaves starting from index `start`.
type FetchLeaves func(start uint64, count uint) ([][]byte, error)

type LeafHashFn func(index uint64, data []byte) []byte

func NewLogVerifier(fetchLeaves FetchLeaves, lh LeafHashFn, ih compact.HashFn, batchHeight uint) LogVerifier {
	return LogVerifier{
		fetchLeaves: fetchLeaves,
		lh:          lh,
		rf:          &compact.RangeFactory{Hash: ih},
		batchSize:   1 << batchHeight,
	}
}

type LogVerifier struct {
	fetchLeaves FetchLeaves
	lh          LeafHashFn
	rf          *compact.RangeFactory
	batchSize   uint
}

func (v LogVerifier) MerkleRoot(size uint64) ([]byte, error) {
	r := v.rf.NewEmptyRange(0)
	for i := uint64(0); i < size; i += uint64(v.batchSize) {
		remaining := size - i
		batchSize := v.batchSize
		if remaining < uint64(batchSize) {
			batchSize = uint(remaining)
		}
		leaves, err := v.fetchLeaves(i, batchSize)
		if err != nil {
			return nil, fmt.Errorf("failed to get leaves from DB: %w", err)
		}
		index := i
		for _, l := range leaves {
			r.Append(v.lh(index, l), nil)
			index++
		}
	}
	return r.GetRootHash(nil)
}
