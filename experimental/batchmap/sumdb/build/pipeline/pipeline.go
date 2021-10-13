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

// Package pipeline contains Beam pipeline library functions for the SumDB
// verifiable map.
package pipeline

import (
	"errors"
	"fmt"

	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"github.com/golang/glog"
	"github.com/google/trillian/experimental/batchmap"
)

// InputLog allows access to entries from the SumDB.
type InputLog interface {
	// Head returns the metadata of available entries.
	Head() (checkpoint []byte, count int64, err error)
	// Entries returns a PCollection of Metadata, containing entries in range [start, end).
	Entries(s beam.Scope, start, end int64) beam.PCollection
}

// InputLogMetadata describes the provenance information of the input
// log to be passed around atomically.
type InputLogMetadata struct {
	Checkpoint []byte
	Entries    int64
}

// MapBuilder contains the static configuration for a map, and allows
// maps at different log sizes to be built using its methods.
type MapBuilder struct {
	source       InputLog
	treeID       int64
	prefixStrata int
	versionLogs  bool
}

// NewMapBuilder returns a MapBuilder for a map with the given configuration.
func NewMapBuilder(source InputLog, treeID int64, prefixStrata int, versionLogs bool) MapBuilder {
	return MapBuilder{
		source:       source,
		treeID:       treeID,
		prefixStrata: prefixStrata,
		versionLogs:  versionLogs,
	}
}

// Create builds a map from scratch, using the first `size` entries in the
// input log. If there aren't enough entries then it will fail.
// It returns a PCollection of *Tile as the first output, and any logs built
// will be output in the second PCollection (of type ModuleVersionLog).
func (b *MapBuilder) Create(s beam.Scope, size int64) (beam.PCollection, beam.PCollection, InputLogMetadata, error) {
	var tiles, logs beam.PCollection

	endID, golden, err := b.getLogEnd(size)
	if err != nil {
		return tiles, logs, InputLogMetadata{}, err
	}

	records := b.source.Entries(s.Scope("source"), 0, endID)
	entries := CreateEntries(s, b.treeID, records)

	if b.versionLogs {
		var logEntries beam.PCollection
		logEntries, logs = MakeVersionLogs(s, b.treeID, records)
		entries = beam.Flatten(s, entries, logEntries)
	}

	glog.Infof("Creating new map revision from range [0, %d)", endID)
	tiles, err = batchmap.Create(s, entries, b.treeID, hash, b.prefixStrata)

	return tiles, logs, InputLogMetadata{
		Checkpoint: golden,
		Entries:    endID,
	}, err
}

// Update builds a map using the last version built, and updating it to
// include all the first `size` entries from the input log. If there aren't
// enough entries then it will fail.
// It returns a PCollection of *Tile as the first output.
func (b *MapBuilder) Update(s beam.Scope, lastTiles beam.PCollection, provenance InputLogMetadata, size int64) (beam.PCollection, InputLogMetadata, error) {
	var tiles beam.PCollection

	if b.versionLogs {
		return tiles, InputLogMetadata{}, errors.New("unsupported: incremental build incompatible with version logs")
	}

	endID, golden, err := b.getLogEnd(size)
	if err != nil {
		return tiles, InputLogMetadata{}, err
	}

	startID := provenance.Entries
	if startID >= endID {
		return tiles, InputLogMetadata{}, fmt.Errorf("startID (%d) >= endID (%d)", startID, endID)
	}

	records := b.source.Entries(s.Scope("source"), startID, endID)
	entries := CreateEntries(s, b.treeID, records)

	glog.Infof("Updating with range [%d, %d)", startID, endID)
	tiles, err = batchmap.Update(s, lastTiles, entries, b.treeID, hash, b.prefixStrata)

	return tiles, InputLogMetadata{
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
