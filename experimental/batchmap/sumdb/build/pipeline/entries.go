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
	"regexp"
	"strings"

	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/register"
	"github.com/google/trillian/experimental/batchmap"
	"github.com/google/trillian/merkle/coniks"
	"github.com/google/trillian/merkle/smt/node"
)

func init() {
	register.Function1x1(ParseStatementFn)
	beam.RegisterType(reflect.TypeOf((*mapEntryFn)(nil)).Elem())
}

const hash = crypto.SHA512_256

// InputLogLeaf is a leaf in an input log, with its sequence index and data.
type InputLogLeaf struct {
	ID   int64
	Data []byte
}

// Metadata is not really metadata, and is in fact the parsed RawCloneLeaf.
// TODO(mhutchinson): rename this.
type Metadata struct {
	ID       int64
	Module   string
	Version  string
	RepoHash string
	ModHash  string
}

var (
	// Example leaf:
	// golang.org/x/text v0.3.0 h1:g61tztE5qeGQ89tm6NTjjM9VPIm088od1l6aSorWRWg=
	// golang.org/x/text v0.3.0/go.mod h1:NqM8EUOU14njkJ3fqMW+pc6Ldnwhi/IjpwHt7yyuwOQ=
	//
	line0RE = regexp.MustCompile(`(.*) (.*) h1:(.*)`)
	line1RE = regexp.MustCompile(`(.*) (.*)/go.mod h1:(.*)`)
)

func ParseStatementFn(l InputLogLeaf) Metadata {
	index := l.ID
	lines := strings.Split(string(l.Data), "\n")

	if len(lines) != 2 {
		panic(fmt.Errorf("Expected 2 lines in log leaf, but got %d", len(lines)))
	}
	line0Parts := line0RE.FindStringSubmatch(lines[0])
	if got, want := len(line0Parts), 4; got != want {
		panic(fmt.Errorf("Regexp: line0 expected %d submatches, but got %d", want, got))
	}
	line0Module, line0Version, line0Hash := line0Parts[1], line0Parts[2], line0Parts[3]

	line1Parts := line1RE.FindStringSubmatch(lines[1])
	if got, want := len(line1Parts), 4; got != want {
		panic(fmt.Errorf("Regexp: line1 expected %d submatches, but got %d (%q)", want, got, lines[1]))
	}
	line1Module, line1Version, line1Hash := line1Parts[1], line1Parts[2], line1Parts[3]

	// TODO(mhutchinson): perhaps this should be handled more elegantly, but it MUST never happen
	if line0Module != line1Module {
		panic(fmt.Errorf("mismatched module names at %d: (%s, %s)", index, line0Module, line1Module))
	}
	if line0Version != line1Version {
		panic(fmt.Errorf("mismatched version names at %d: (%s, %s)", index, line0Version, line1Version))
	}

	return Metadata{
		ID:       index,
		Module:   line0Module,
		Version:  line0Version,
		RepoHash: line0Hash,
		ModHash:  line1Hash,
	}
}

// ParseLogInputs converts the PCollection<InputLogLeaf> into a PCollection<Metadata>.
func ParseLogInputs(s beam.Scope, logInputs beam.PCollection) beam.PCollection {
	return beam.ParDo(s.Scope("parseStatements"), ParseStatementFn, logInputs)
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
