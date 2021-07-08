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

package ftmap

import (
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/apache/beam/sdks/go/pkg/beam"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian/experimental/batchmap"
	"github.com/google/trillian/merkle/coniks"
	"github.com/google/trillian/merkle/smt/node"
)

func init() {
	beam.RegisterFunction(aggregationFn)
	beam.RegisterType(reflect.TypeOf((*api.AggregatedFirmware)(nil)).Elem())
	beam.RegisterType(reflect.TypeOf((*aggregatedFirmwareHashFn)(nil)).Elem())
}

// Aggregate will output an entry for each firmware entry in the input log, which includes the
// firmware metadata, along with an aggregated representation of the annotations for it. The
// rules are:
// * AnnotationMalware: `Good` is true providing there are no malware annotations that claim the
//                      firmware is bad.
func Aggregate(s beam.Scope, treeID int64, fws, annotationMalwares beam.PCollection) (beam.PCollection, beam.PCollection) {
	keyedFws := beam.ParDo(s, func(l *firmwareLogEntry) (uint64, *firmwareLogEntry) { return uint64(l.Index), l }, fws)
	keyedAnns := beam.ParDo(s, func(a *annotationMalwareLogEntry) (uint64, *annotationMalwareLogEntry) {
		return a.Annotation.FirmwareID.LogIndex, a
	}, annotationMalwares)
	annotations := beam.ParDo(s, aggregationFn, beam.CoGroupByKey(s, keyedFws, keyedAnns))
	return beam.ParDo(s, &aggregatedFirmwareHashFn{treeID}, annotations), annotations
}

func aggregationFn(fwIndex uint64, fwit func(**firmwareLogEntry) bool, amit func(**annotationMalwareLogEntry) bool) (*api.AggregatedFirmware, error) {
	// There will be exactly one firmware entry for the log index.
	var fwle *firmwareLogEntry
	if !fwit(&fwle) {
		return nil, fmt.Errorf("aggregationFn for %d found no firmware", fwIndex)
	}

	// And 0-many annotations. The FW is good as long as no annotations say that it is not.
	good := true
	var amle *annotationMalwareLogEntry
	for amit(&amle) {
		good = good && amle.Annotation.Good
	}

	return &api.AggregatedFirmware{
		Index: fwIndex,
		Good:  good,
	}, nil
}

type aggregatedFirmwareHashFn struct {
	TreeID int64
}

func (fn *aggregatedFirmwareHashFn) ProcessElement(afw *api.AggregatedFirmware) (*batchmap.Entry, error) {
	// We use the Index of the FW Log Entry as the key because that's unique and what we matched on.
	// The client already has this information from the inclusion proof so it's known information.
	// Using `afw.Firmware.FirmwareImageSHA512` is another logical choice, but there's nothing to
	// prevent multiple entries in the log with the same FW hash, and thus we'd have key conflicts.
	// If we did want to use image hash as the key then we'd need to restructure FW annotations to
	// remove the `LogIndex` from `FirmwareId`.
	key := []byte(fmt.Sprintf("summary:%d", afw.Index))
	value, err := json.Marshal(afw)
	if err != nil {
		return nil, fmt.Errorf("marshaling AggregatedFirmware failed: %v", err)
	}

	kbs := sha512.Sum512_256(key)
	leafID := node.NewID(string(kbs[:]), 256)
	return &batchmap.Entry{
		HashKey:   kbs[:],
		HashValue: coniks.Default.HashLeaf(fn.TreeID, leafID, value),
	}, nil
}
