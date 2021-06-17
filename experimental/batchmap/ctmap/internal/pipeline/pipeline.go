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

// Package pipeline contains Beam pipeline library functions for the CT
// verifiable map.
package pipeline

import (
	"context"
	"crypto"
	"fmt"
	"reflect"

	"github.com/apache/beam/sdks/go/pkg/beam"
	"github.com/golang/glog"
	ct "github.com/google/certificate-transparency-go"
	"github.com/google/certificate-transparency-go/tls"
	"github.com/google/certificate-transparency-go/x509"
	"github.com/google/trillian/experimental/batchmap"
)

const treeID = int64(12345)

var (
	cntCertsWithNoDomain = beam.NewCounter("ctmap", "certs-zero-domains")
	cntPrecerts          = beam.NewCounter("ctmap", "precerts")
	cntLeavesProcessed   = beam.NewCounter("ctmap", "leaves-processed")
)

func init() {
	beam.RegisterType(reflect.TypeOf((*domainEntry)(nil)).Elem())
}

// InputLog allows access to entries from the log.
type InputLog interface {
	// Head returns the metadata of available entries.
	Head(ctx context.Context) (checkpoint []byte, count int64, err error)

	// Entries returns a PCollection of InputLogLeaf, containing entries in range [start, end).
	Entries(s beam.Scope, start, end int64) beam.PCollection
}

// InputLogLeaf is a leaf in an input log, with its sequence index and data.
type InputLogLeaf struct {
	Seq  int64
	Data []byte
}

// InputLogMetadata describes the provenance information of the input
// log to be passed around atomically.
type InputLogMetadata struct {
	Checkpoint []byte
	Entries    int64
}

// Result is returned on successful run of the pipeline. It primarily
// exists to name the output and aid readability, as PCollections are untyped
// in code, so having them as named fields at least aids a little.
type Result struct {
	// MapTiles is a PCollection of *batchmap.Tile.
	MapTiles beam.PCollection
	// DomainCounts is a PCollection of *DomainCertIndexLog.
	DomainCertIndexLogs beam.PCollection
	Metadata            InputLogMetadata
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
func (b *MapBuilder) Create(ctx context.Context, s beam.Scope, size int64) (Result, error) {
	var r Result

	endID, golden, err := b.getLogEnd(ctx, size)
	if err != nil {
		return r, err
	}

	// TODO(mhutchinson): Find a better hack to parallize data source.
	batchSize := size/10 + 1

	rawLeaves := make([]beam.PCollection, 0, endID/batchSize)
	for i := int64(0); i < endID; i += batchSize {
		end := i + batchSize
		if end > endID {
			end = endID
		}
		rawLeaves = append(rawLeaves, b.source.Entries(s.Scope("source"), i, end))
	}
	domains := beam.ParDo(s.Scope("keyByDomain"), rawLeafToDomainEntries, beam.Flatten(s, rawLeaves...))

	entries, logs := MakeDomainLogs(s.Scope("MakeDomainLogs"), treeID, domains)

	glog.Infof("Creating new map revision from range [0, %d)", endID)
	tiles, err := batchmap.Create(s, entries, b.treeID, crypto.SHA512_256, b.prefixStrata)

	return Result{
		MapTiles:            tiles,
		DomainCertIndexLogs: logs,
		Metadata: InputLogMetadata{
			Checkpoint: golden,
			Entries:    endID,
		},
	}, err
}

func (b *MapBuilder) getLogEnd(ctx context.Context, requiredEntries int64) (int64, []byte, error) {
	golden, totalLeaves, err := b.source.Head(ctx)
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

func rawLeafToDomainEntries(ctx context.Context, rawLeaf InputLogLeaf, emit func(*domainEntry)) error {
	cntLeavesProcessed.Inc(ctx, 1)
	var leaf ct.MerkleTreeLeaf
	if rest, err := tls.Unmarshal(rawLeaf.Data, &leaf); err != nil {
		return fmt.Errorf("failed to unmarshal MerkleTreeLeaf: %v", err)
	} else if len(rest) > 0 {
		return fmt.Errorf("MerkleTreeLeaf: trailing data %d bytes", len(rest))
	}

	var cert *x509.Certificate
	var err error
	switch eType := leaf.TimestampedEntry.EntryType; eType {
	case ct.X509LogEntryType:
		cert, err = leaf.X509Certificate()
		if x509.IsFatal(err) {
			return fmt.Errorf("failed to parse certificate: %v", err)
		}

	case ct.PrecertLogEntryType:
		cntPrecerts.Inc(ctx, 1)
		cert, err = leaf.Precertificate()
		if x509.IsFatal(err) {
			return fmt.Errorf("failed to parse precertificate: %v", err)
		}

	default:
		return fmt.Errorf("unknown entry type: %v", eType)
	}

	if len(cert.DNSNames) == 0 {
		cntCertsWithNoDomain.Inc(ctx, 1)
	}
	for _, n := range cert.DNSNames {
		emit(&domainEntry{
			Index:   uint64(rawLeaf.Seq),
			DNSName: n,
		})
	}
	return nil
}

type domainEntry struct {
	Index   uint64
	DNSName string
}
