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
	"strings"
	"testing"

	"github.com/google/trillian-examples/witness/golang/internal/persistence"
	ptest "github.com/google/trillian-examples/witness/golang/internal/persistence/testonly"
	_ "github.com/mattn/go-sqlite3" // Load drivers for sqlite3
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var nopClose = func() error { return nil }

func TestGetLogs(t *testing.T) {
	ptest.TestGetLogs(t, func() (persistence.LogStatePersistence, func() error) {
		return NewPersistence(), nopClose
	})
}

func TestWriteOps(t *testing.T) {
	ptest.TestWriteOps(t, func() (persistence.LogStatePersistence, func() error) {
		return NewPersistence(), nopClose
	})
}

func TestWriteOpsConcurrent(t *testing.T) {
	p := NewPersistence()

	g := errgroup.Group{}

	for i := 0; i < 25; i++ {
		i := i
		g.Go(func() error {
			w, err := p.WriteOps("foo")
			if err != nil {
				return fmt.Errorf("WriteOps %d: %v", i, err)
			}
			defer w.Close()
			if _, _, err := w.GetLatest(); err != nil {
				if status.Code(err) != codes.NotFound {
					return fmt.Errorf("GetLatest %d: %v", i, err)
				}
			}
			// Ignore any error on Set because we expect some.
			w.Set([]byte(fmt.Sprintf("success %d", i)), nil)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		t.Error(err)
	}

	r, err := p.ReadOps("foo")
	if err != nil {
		t.Fatal(err)
	}
	cp, _, err := r.GetLatest()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(cp), "success") {
		t.Errorf("expected at least one success but got %s", string(cp))
	}
}
