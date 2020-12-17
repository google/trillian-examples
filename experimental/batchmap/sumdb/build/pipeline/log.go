package pipeline

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/apache/beam/sdks/go/pkg/beam"

	"github.com/google/trillian/experimental/batchmap"
	"github.com/google/trillian/merkle/compact"

	"golang.org/x/mod/sumdb/tlog"
)

func init() {
	beam.RegisterType(reflect.TypeOf((*makeVersionListFn)(nil)).Elem())
}

func MakeVersionList(s beam.Scope, metadata beam.PCollection) beam.PCollection {
	keyed := beam.ParDo(s, func(m Metadata) (string, Metadata) { return m.Module, m }, metadata)
	return beam.ParDo(s, &makeVersionListFn{}, beam.GroupByKey(s, keyed))
}

type makeVersionListFn struct {
	rf *compact.RangeFactory
}

func (fn *makeVersionListFn) Setup() {
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

func (fn *makeVersionListFn) ProcessElement(module string, metadata func(*Metadata) bool) (*batchmap.Entry, error) {
	// We need to ensure ordering by ID in the original log for stability.

	// First build up a map from ID to hash.
	mm := make(map[int64][]byte)
	var m Metadata
	for metadata(&m) {
		h := hash.New()
		h.Write([]byte(m.Version))

		mm[m.ID] = h.Sum(nil)
	}

	// Now order the keyset.
	keys := make([]int64, 0, len(mm))
	for k := range mm {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	// Now iterate the map in the right order to build the log.
	logRange := fn.rf.NewEmptyRange(0)
	for _, id := range keys {
		logRange.Append(mm[id], nil)
	}

	// Construct the map entry for this module version log.
	logRoot, err := logRange.GetRootHash(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create log for %q: %v", module, err)
	}
	h := hash.New()
	h.Write([]byte(module))
	return &batchmap.Entry{
		HashKey:   h.Sum(nil),
		HashValue: logRoot,
	}, nil
}
