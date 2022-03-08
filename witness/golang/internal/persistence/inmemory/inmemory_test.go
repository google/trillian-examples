// Copyright 2022 Google LLC. All Rights Reserved.
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

package inmemory

import (
	"fmt"
	"testing"

	"github.com/google/trillian-examples/witness/golang/internal/persistence"
	ptest "github.com/google/trillian-examples/witness/golang/internal/persistence/testonly"
	_ "github.com/mattn/go-sqlite3" // Load drivers for sqlite3
)

var nopClose = func() error { return nil }

func TestGetLogs(t *testing.T) {
	ptest.TestGetLogs(t, func() (persistence.LogStatePersistence, func() error) {
		return NewInMemoryPersistence(), nopClose
	})
}

func TestWriteOps(t *testing.T) {
	ptest.TestWriteOps(t, func() (persistence.LogStatePersistence, func() error) {
		return NewInMemoryPersistence(), nopClose
	})
}

func TestWriteOpsAdvanced(t *testing.T) {
	p := NewInMemoryPersistence()

	for i := 0; i < 10; i++ {
		fooWrite, err := p.WriteOps("foo")
		if err != nil {
			t.Fatal(err)
		}
		conflictWrite, err := p.WriteOps("foo")
		if err != nil {
			t.Fatal(err)
		}

		if err := fooWrite.SetCheckpoint([]byte(fmt.Sprintf("success %d", i)), nil); err != nil {
			t.Fatal(err)
		}
		if err := conflictWrite.SetCheckpoint([]byte(fmt.Sprintf("fail %d", i)), nil); err != nil {
			t.Fatal(err)
		}

		if err := fooWrite.Commit(); err != nil {
			t.Fatal(err)
		}
		if err := conflictWrite.Commit(); err == nil {
			t.Fatal("expected error on conflicting write")
		}
	}

	read, err := p.ReadOps("foo")
	if err != nil {
		t.Fatal(err)
	}
	cp, _, _ := read.GetLatestCheckpoint()
	if got, want := string(cp), "success 9"; got != want {
		t.Errorf("got != want (%s != %s)", got, want)
	}
}
