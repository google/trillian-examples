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
func MakeVersionLogs(s beam.Scope, metadata beam.PCollection) (beam.PCollection, beam.PCollection) {
	keyed := beam.ParDo(s, func(m Metadata) (string, Metadata) { return m.Module, m }, metadata)
	return beam.ParDo2(s, &makeVersionListFn{}, beam.GroupByKey(s, keyed))
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

func (fn *makeVersionListFn) ProcessElement(module string, metadata func(*Metadata) bool, emitEntry func(*batchmap.Entry), emitLog func(*ModuleVersionLog)) error {
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
	logRange := fn.rf.NewEmptyRange(0)
	versions := make([]string, 0, len(keys))
	for _, id := range keys {
		v := mm[id]
		versions = append(versions, v)
		h := hash.New()
		h.Write([]byte(v))
		logRange.Append(h.Sum(nil), nil)
	}

	// Construct the map entry for this module version log.
	logRoot, err := logRange.GetRootHash(nil)
	if err != nil {
		return fmt.Errorf("failed to create log for %q: %v", module, err)
	}
	h := hash.New()
	h.Write([]byte(module))

	emitEntry(&batchmap.Entry{
		HashKey:   h.Sum(nil),
		HashValue: logRoot,
	})
	emitLog(
		&ModuleVersionLog{
			Module:   module,
			Versions: versions,
		})
	return nil
}
