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

package pipeline

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"reflect"
	"sort"

	"github.com/apache/beam/sdks/v2/go/pkg/beam"

	"github.com/google/trillian/experimental/batchmap"
	"github.com/google/trillian/merkle/coniks"
	"github.com/google/trillian/merkle/smt/node"
	"github.com/transparency-dev/merkle/compact"
)

func init() {
	beam.RegisterFunction(makeDomainLogFn)
	beam.RegisterType(reflect.TypeOf((*moduleLogHashFn)(nil)).Elem())
	beam.RegisterType(reflect.TypeOf((*DomainCertIndexLog)(nil)).Elem())
}

// MakeDomainLogs takes all domainEntry records, groups them by domain,
// and then creates a log containing all of the indices (in order) for which
// a cert relates to that domain. This method returns two PCollections:
// 1. the first is of type Entry; the key/value data to include in the map
// 2. the second is of type DomainCertIndexLog.
func MakeDomainLogs(s beam.Scope, treeID int64, logEntries beam.PCollection) (beam.PCollection, beam.PCollection) {
	keyed := beam.ParDo(s, func(l *domainEntry) (string, *domainEntry) { return l.DNSName, l }, logEntries)
	logs := beam.ParDo(s, makeDomainLogFn, beam.GroupByKey(s, keyed))
	return beam.ParDo(s, &moduleLogHashFn{TreeID: treeID}, logs), logs
}

// TODO(mhutchinson): extract library for creating logs in Beam.
type moduleLogHashFn struct {
	TreeID int64

	rf *compact.RangeFactory
}

func (fn *moduleLogHashFn) Setup() {
	fn.rf = &compact.RangeFactory{
		Hash: NodeHash,
	}
}

func (fn *moduleLogHashFn) ProcessElement(log *DomainCertIndexLog) (*batchmap.Entry, error) {
	logRange := fn.rf.NewEmptyRange(0)
	for _, v := range log.Indices {
		bs := make([]byte, 8)
		binary.LittleEndian.PutUint64(bs, v)
		h := RecordHash(bs)
		logRange.Append(h[:], nil)
	}
	logRoot, err := logRange.GetRootHash(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create log for %q: %v", log.Domain, err)
	}
	logKey := sha256.Sum256([]byte(log.Domain))

	leafID := node.NewID(string(logKey[:]), uint(len(logKey)*8))
	return &batchmap.Entry{
		HashKey:   logKey[:],
		HashValue: coniks.Default.HashLeaf(fn.TreeID, leafID, logRoot),
	}, nil
}

func makeDomainLogFn(domain string, lit func(**domainEntry) bool) (*DomainCertIndexLog, error) {
	// We need to ensure ordering by sequence ID in the original log for stability.

	// First consume the iterator into an in-memory list.
	// This puts the whole record in the list for now, but this can be optimized to only store
	// the revision ID.
	entries := make([]*domainEntry, 0)
	var e *domainEntry
	for lit(&e) {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Index < entries[j].Index })

	indices := make([]uint64, len(entries))
	for i := range entries {
		indices[i] = entries[i].Index
	}

	return &DomainCertIndexLog{
		Domain:  domain,
		Indices: indices,
	}, nil
}

var zeroPrefix = []byte{0x00}
var onePrefix = []byte{0x01}

// RecordHash returns the content hash for the given record data.
func RecordHash(data []byte) []byte {
	// SHA256(0x00 || data)
	h := sha256.New()
	h.Write(zeroPrefix)
	h.Write(data)
	return h.Sum(nil)
}

// NodeHash returns the hash for an interior tree node with the given left and right hashes.
func NodeHash(left, right []byte) []byte {
	// SHA256(0x01 || left || right)
	h := sha256.New()
	h.Write(onePrefix)
	h.Write(left)
	h.Write(right)
	return h.Sum(nil)
}

type DomainCertIndexLog struct {
	Domain  string
	Indices []uint64
}
