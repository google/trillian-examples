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
	"crypto"
	"fmt"
	"reflect"

	"github.com/apache/beam/sdks/go/pkg/beam"
	"github.com/google/trillian/experimental/batchmap"
	"github.com/google/trillian/merkle/coniks"
	"github.com/google/trillian/merkle/smt/node"
)

func init() {
	beam.RegisterType(reflect.TypeOf((*mapEntryFn)(nil)).Elem())
}

const hash = crypto.SHA512_256

// Metadata is the audit.Metadata object with the addition of an ID field.
// It must map to the scheme of the leafMetadata table.
type Metadata struct {
	ID       int64
	Module   string
	Version  string
	RepoHash string
	ModHash  string
}

// CreateEntries converts the PCollection<Metadata> into a PCollection<Entry> that will be
// committed to by the map.
func CreateEntries(s beam.Scope, treeID int64, records beam.PCollection) beam.PCollection {
	return beam.ParDo(s.Scope("mapentries"), &mapEntryFn{treeID}, records)
}

type mapEntryFn struct {
	TreeID int64
}

func (fn *mapEntryFn) ProcessElement(m Metadata, emit func(*batchmap.Entry)) {
	h := hash.New()
	h.Write([]byte(fmt.Sprintf("%s %s/go.mod", m.Module, m.Version)))
	modKey := h.Sum(nil)
	modLeafID := node.NewID(string(modKey), uint(len(modKey)*8))

	emit(&batchmap.Entry{
		HashKey:   modKey,
		HashValue: coniks.Default.HashLeaf(fn.TreeID, modLeafID, []byte(m.ModHash)),
	})

	h = hash.New()
	h.Write([]byte(fmt.Sprintf("%s %s", m.Module, m.Version)))
	repoKey := h.Sum(nil)
	repoLeafID := node.NewID(string(repoKey), uint(len(repoKey)*8))

	emit(&batchmap.Entry{
		HashKey:   repoKey,
		HashValue: coniks.Default.HashLeaf(fn.TreeID, repoLeafID, []byte(m.RepoHash)),
	})
}
