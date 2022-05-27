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

	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian/experimental/batchmap"
)

func init() {
	beam.RegisterFunction(parseStatementFn)
	beam.RegisterFunction(parseFirmwareFn)
	beam.RegisterFunction(parseAnnotationMalwareFn)
	beam.RegisterFunction(partitionFn)
	beam.RegisterType(reflect.TypeOf((*InputLogMetadata)(nil)).Elem())
	beam.RegisterType(reflect.TypeOf((*InputLogLeaf)(nil)).Elem())
	beam.RegisterType(reflect.TypeOf((*PipelineResult)(nil)).Elem())
	beam.RegisterType(reflect.TypeOf((*loggedStatement)(nil)).Elem())
	beam.RegisterType(reflect.TypeOf((*firmwareLogEntry)(nil)).Elem())
	beam.RegisterType(reflect.TypeOf((*annotationMalwareLogEntry)(nil)).Elem())
}

// InputLog allows access to entries from the FT Log.
type InputLog interface {
	// Head returns the metadata of available entries.
	// The log checkpoint is a serialized LogRootV1.
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

// PipelineResult is returned on successful run of the pipeline. It primarily
// exists to name the output and aid readability, as PCollections are untyped
// in code, so having them as named fields at least aids a little.
type PipelineResult struct {
	MapTiles           beam.PCollection
	DeviceLogs         beam.PCollection
	AggregatedFirmware beam.PCollection
	Metadata           InputLogMetadata
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
// The results of the pipeline on success are returned as a PipelineResult.
func (b *MapBuilder) Create(s beam.Scope, size int64) (*PipelineResult, error) {
	var tiles, logs beam.PCollection

	endID, golden, err := b.getLogEnd(size)
	if err != nil {
		return nil, err
	}

	// Read the log as a collection of InputLogLeaf.
	inputLeaves := b.source.Entries(s.Scope("source"), 0, endID)
	// Parse these into their loggedStatements.
	statements := beam.ParDo(s.Scope("parseStatement"), parseStatementFn, inputLeaves)

	// Partition into:
	// 0: FW Metadata
	// 1: Annotation malware
	// 2: Everything else
	partitions := beam.Partition(s.Scope("partition"), MaxPartitions, partitionFn, statements)

	fws := beam.ParDo(s, parseFirmwareFn, partitions[FirmwareMetaPartition])
	ams := beam.ParDo(s, parseAnnotationMalwareFn, partitions[MalwareStatementPartition])

	// Branch 1: create the logs of firmware releases.
	logEntries, logs := MakeReleaseLogs(s.Scope("makeLogs"), b.treeID, fws)

	// Branch 2: aggregate firmware releases with their annotations.
	annotationEntries, aggregated := Aggregate(s, b.treeID, fws, ams)

	// Flatten the entries together to create a single unified map.
	entries := beam.Flatten(s, logEntries, annotationEntries)

	glog.Infof("Creating new map revision from range [0, %d)", endID)
	tiles, err = batchmap.Create(s, entries, b.treeID, crypto.SHA512_256, b.prefixStrata)

	return &PipelineResult{
		MapTiles:           tiles,
		DeviceLogs:         logs,
		AggregatedFirmware: aggregated,
		Metadata: InputLogMetadata{
			Checkpoint: golden,
			Entries:    endID,
		},
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

const (
	// FirmwareMetaPartition is the partition index for partition containing FirmwareMetadata
	FirmwareMetaPartition = iota
	// MalwareStatementPartition is the partition index for partition containing MalwareStatement
	MalwareStatementPartition
	// UnclassifiedStatementPartition is the partition index for partition containing anything not classified above
	UnclassifiedStatementPartition
	// MaxPartitions is the total number of supported partitions. Add new partitions here.
	MaxPartitions
)

// partitionFn partitions the statements by type.
func partitionFn(s *loggedStatement) int {
	switch s.Statement.Type {
	case api.FirmwareMetadataType:
		return FirmwareMetaPartition
	case api.MalwareStatementType:
		return MalwareStatementPartition
	default:
		return UnclassifiedStatementPartition
	}
}

// loggedStatement is a parsed version of InputLogLeaf.
type loggedStatement struct {
	Index     int64
	Statement api.SignedStatement
}

func parseStatementFn(l InputLogLeaf) (*loggedStatement, error) {
	var s api.SignedStatement
	if err := json.Unmarshal(l.Data, &s); err != nil {
		return nil, err
	}

	return &loggedStatement{
		Index:     l.Seq,
		Statement: s,
	}, nil
}

type firmwareLogEntry struct {
	Index    int64
	Firmware api.FirmwareMetadata
}

func parseFirmwareFn(s *loggedStatement) (*firmwareLogEntry, error) {
	var m api.FirmwareMetadata
	if err := json.Unmarshal(s.Statement.Statement, &m); err != nil {
		return nil, err
	}
	return &firmwareLogEntry{
		Index:    s.Index,
		Firmware: m,
	}, nil
}

type annotationMalwareLogEntry struct {
	Index      int64
	Annotation api.MalwareStatement
}

func parseAnnotationMalwareFn(s *loggedStatement) (*annotationMalwareLogEntry, error) {
	var a api.MalwareStatement
	if err := json.Unmarshal(s.Statement.Statement, &a); err != nil {
		return nil, err
	}
	return &annotationMalwareLogEntry{
		Index:      s.Index,
		Annotation: a,
	}, nil
}
