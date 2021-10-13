// Copyright 2020 Google LLC. All Rights Reserved.
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

package pipeline

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/apache/beam/sdks/v2/go/pkg/beam"

	"github.com/google/trillian/experimental/batchmap"
	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/merkle/coniks"
	"github.com/google/trillian/merkle/smt/node"

	"golang.org/x/mod/sumdb/tlog"
)

func init() {
	beam.RegisterFunction(makeModuleVersionLogFn)
	beam.RegisterType(reflect.TypeOf((*moduleLogHashFn)(nil)).Elem())
}

// ModuleVersionLog represents the versions found for a single
// Go Module within the SumDB log. The versions are sorted by
// the order they are logged in SumDB.
type ModuleVersionLog struct {
	Module   string
	Versions []string
}

// MakeVersionLogs takes the Metadata for all modules and processes this by
// module in order to create logs of versions. The versions for each module
// are sorted (by ID in the original log), and a log is constructed for each
// module. This method returns two PCollections: the first is of type Entry
// and is the key/value data to include in the map, the second is of type
// ModuleVersionLog.
func MakeVersionLogs(s beam.Scope, treeID int64, metadata beam.PCollection) (beam.PCollection, beam.PCollection) {
	keyed := beam.ParDo(s, func(m Metadata) (string, Metadata) { return m.Module, m }, metadata)
	logs := beam.ParDo(s, makeModuleVersionLogFn, beam.GroupByKey(s, keyed))
	return beam.ParDo(s, &moduleLogHashFn{TreeID: treeID}, logs), logs
}

type moduleLogHashFn struct {
	TreeID int64

	rf *compact.RangeFactory
}

func (fn *moduleLogHashFn) Setup() {
	fn.rf = &compact.RangeFactory{
		Hash: func(left, right []byte) []byte {
			// There is no particular need for using this hash function, but it was convenient.
			var lHash, rHash tlog.Hash
			copy(lHash[:], left)
			copy(rHash[:], right)
			thash := tlog.NodeHash(lHash, rHash)
			return thash[:]
		},
	}
}

func (fn *moduleLogHashFn) ProcessElement(log *ModuleVersionLog) (*batchmap.Entry, error) {
	logRange := fn.rf.NewEmptyRange(0)
	for _, v := range log.Versions {
		h := tlog.RecordHash([]byte(v))
		logRange.Append(h[:], nil)
	}
	logRoot, err := logRange.GetRootHash(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create log for %q: %v", log.Module, err)
	}
	h := hash.New()
	h.Write([]byte(log.Module))
	logKey := h.Sum(nil)
	leafID := node.NewID(string(logKey), uint(len(logKey)*8))

	return &batchmap.Entry{
		HashKey:   h.Sum(nil),
		HashValue: coniks.Default.HashLeaf(fn.TreeID, leafID, logRoot),
	}, nil
}

func makeModuleVersionLogFn(module string, metadata func(*Metadata) bool) (*ModuleVersionLog, error) {
	// We need to ensure ordering by ID in the original log for stability.

	// First build up a map from ID to version.
	mm := make(map[int64]string)
	var m Metadata
	for metadata(&m) {
		mm[m.ID] = m.Version
	}

	// Now order the keyset.
	keys := make([]int64, 0, len(mm))
	for k := range mm {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	// Now iterate the map in the right order to build the log.
	versions := make([]string, 0, len(keys))
	for _, id := range keys {
		versions = append(versions, mm[id])
	}

	return &ModuleVersionLog{
		Module:   module,
		Versions: versions,
	}, nil
}
