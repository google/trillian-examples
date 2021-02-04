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

// Package ftmap contains Beam pipeline library functions for the FT
// verifiable map.
package ftmap

import (
	"crypto"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/apache/beam/sdks/go/pkg/beam"
	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian/experimental/batchmap"
)

func init() {
	beam.RegisterFunction(parseLogEntryFn)
	beam.RegisterType(reflect.TypeOf((*InputLogMetadata)(nil)).Elem())
	beam.RegisterType(reflect.TypeOf((*InputLogLeaf)(nil)).Elem())
	beam.RegisterType(reflect.TypeOf((*firmwareLogEntry)(nil)).Elem())
}

// InputLog allows access to entries from the FT Log.
type InputLog interface {
	// Head returns the metadata of available entries.
	Head() (checkpoint []byte, count int64, err error)
	// Entries returns a PCollection of InputLogLeaf, containing entries in range [start, end).
	Entries(s beam.Scope, start, end int64) beam.PCollection
}

// InputLogMetadata describes the provenance information of the input
// log to be passed around atomically.
type InputLogMetadata struct {
	Checkpoint []byte
	Entries    int64
}

// InputLogLeaf is a leaf in an input log, with its sequence index and data.
type InputLogLeaf struct {
	Seq  int64
	Data []byte
}

// MapBuilder contains the static configuration for a map, and allows
// maps at different log sizes to be built using its methods.
type MapBuilder struct {
	source       InputLog
	treeID       int64
	prefixStrata int
}

// NewMapBuilder returns a MapBuilder for a map with the given configuration.
func NewMapBuilder(source InputLog, treeID int64, prefixStrata int) MapBuilder {
	return MapBuilder{
		source:       source,
		treeID:       treeID,
		prefixStrata: prefixStrata,
	}
}

// Create builds a map from scratch, using the first `size` entries in the
// input log. If there aren't enough entries then it will fail.
// It returns a PCollection of *Tile as the first output, and any logs built
// will be output in the second PCollection (of type DeviceReleaseLog).
func (b *MapBuilder) Create(s beam.Scope, size int64) (beam.PCollection, beam.PCollection, InputLogMetadata, error) {
	var tiles, logs beam.PCollection

	endID, golden, err := b.getLogEnd(size)
	if err != nil {
		return tiles, logs, InputLogMetadata{}, err
	}

	inputLeaves := b.source.Entries(s.Scope("source"), 0, endID)
	fws := beam.ParDo(s.Scope("parse"), parseLogEntryFn, inputLeaves)
	entries, logs := MakeReleaseLogs(s.Scope("makeLogs"), b.treeID, fws)

	glog.Infof("Creating new map revision from range [0, %d)", endID)
	tiles, err = batchmap.Create(s, entries, b.treeID, crypto.SHA512_256, b.prefixStrata)

	return tiles, logs, InputLogMetadata{
		Checkpoint: golden,
		Entries:    endID,
	}, err
}

func (b *MapBuilder) getLogEnd(requiredEntries int64) (int64, []byte, error) {
	golden, totalLeaves, err := b.source.Head()
	if err != nil {
		return 0, nil, fmt.Errorf("failed to get Head of input log: %v", err)
	}

	if requiredEntries < 0 {
		return totalLeaves, golden, nil
	}

	if totalLeaves < requiredEntries {
		return 0, nil, fmt.Errorf("wanted %d leaves but only %d available", requiredEntries, totalLeaves)
	}

	return requiredEntries, golden, nil
}

// firmwareLogEntry is a parsed version of InputLogLeaf.
type firmwareLogEntry struct {
	Index    int64
	Firmware api.FirmwareMetadata
}

func parseLogEntryFn(l InputLogLeaf, emit func(*firmwareLogEntry)) error {
	var s api.SignedStatement
	if err := json.Unmarshal(l.Data, &s); err != nil {
		return err
	}

	if s.Type != api.FirmwareMetadataType {
		// TODO(mhutchinson): Support annotation types.
		return nil
	}
	var m api.FirmwareMetadata
	if err := json.Unmarshal(s.Statement, &m); err != nil {
		return err
	}
	emit(&firmwareLogEntry{
		Index:    l.Seq,
		Firmware: m,
	})
	return nil
}
