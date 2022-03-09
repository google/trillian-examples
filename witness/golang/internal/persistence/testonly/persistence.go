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

package persistence

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian-examples/witness/golang/internal/persistence"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGetLogs(t *testing.T, lspFactory func() (persistence.LogStatePersistence, func() error)) {
	lsp, close := lspFactory()
	defer close()
	if err := lsp.Init(); err != nil {
		t.Fatalf("Init(): %v", err)
	}
	if logs, err := lsp.Logs(); err != nil {
		t.Errorf("Logs(): %v", err)
	} else if got, want := len(logs), 0; got != want {
		t.Errorf("got %d logs, want %d", got, want)
	}

	if err := writeCheckpoint(lsp, "foo"); err != nil {
		t.Fatal(err)
	}

	if logs, err := lsp.Logs(); err != nil {
		t.Errorf("Logs(): %v", err)
	} else if got, want := logs, []string{"foo"}; !cmp.Equal(got, want) {
		t.Errorf("got != want (%v != %v)", got, want)
	}
}

func TestWriteOps(t *testing.T, lspFactory func() (persistence.LogStatePersistence, func() error)) {
	lsp, close := lspFactory()
	defer close()
	if err := lsp.Init(); err != nil {
		t.Fatalf("Init(): %v", err)
	}

	read, err := lsp.ReadOps("foo")
	if err != nil {
		t.Fatalf("ReadOps(): %v", err)
	}
	_, _, err = read.GetLatestCheckpoint()
	if got, want := status.Code(err), codes.NotFound; got != want {
		t.Fatalf("error code got != want (%s, %s): %v", got, want, err)
	}

	if err := writeCheckpoint(lsp, "foo"); err != nil {
		t.Fatal(err)
	}

	read, err = lsp.ReadOps("foo")
	if err != nil {
		t.Fatalf("ReadOps(): %v", err)
	}
	var cpRaw []byte
	if cpRaw, _, err = read.GetLatestCheckpoint(); err != nil {
		t.Fatalf("GetLatestCheckpoint(): %v", err)
	}
	if got, want := cpRaw, []byte("foo cp"); !bytes.Equal(got, want) {
		t.Errorf("got != want (%s != %s)", got, want)
	}
}

func writeCheckpoint(lsp persistence.LogStatePersistence, id string) error {
	writeOps, err := lsp.WriteOps(id)
	if err != nil {
		return fmt.Errorf("WriteOps(%s): %v", id, err)
	}
	defer writeOps.Close()
	if err := writeOps.SetCheckpoint([]byte(fmt.Sprintf("%s cp", id)), nil); err != nil {
		return fmt.Errorf("SetCheckpoint(%s): %v", id, err)
	}
	return nil
}
