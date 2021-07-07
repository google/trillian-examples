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

// package testdata contains testdata for the serverless log.
package testdata

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/trillian-examples/serverless/api/layout"
	"github.com/google/trillian-examples/serverless/client"
	"golang.org/x/mod/sumdb/note"
)

const (
	TestLogSecretKey = "PRIVATE+KEY+astra+cad5a3d2+ASgwwenlc0uuYcdy7kI44pQvuz1fw8cS5NqS8RkZBXoy"
	TestLogPublicKey = "astra+cad5a3d2+AZJqeuyE/GnknsCNh1eCtDtwdAwKBddOlS8M2eI1Jt4b"
)

var (
	testdataDir = locateTestdata()
)

// LogSigVerifier returns a note.Verifier for the testdata log's public key.
func LogSigVerifier(t *testing.T) note.Verifier {
	v, err := note.NewVerifier(TestLogPublicKey)
	if err != nil {
		t.Fatalf("Failed to create test log sig verifier: %v", err)
	}
	return v
}

// LogSigner returns a note.Signer for the testdata log's secret key.
func LogSigner(t *testing.T) note.Signer {
	s, err := note.NewSigner(TestLogSecretKey)
	if err != nil {
		t.Fatalf("Failed to create test log signer: %v", err)
	}
	return s
}

// Fetcher returns a client.Fetcher for the testdata log.
func Fetcher() client.Fetcher {
	return func(_ context.Context, p string) ([]byte, error) {
		path := filepath.Join(testdataDir, "log", p)
		return ioutil.ReadFile(path)
	}
}

// HistoryFetcher is a client.Fetcher helper which can be instructed to
// return a particular size checkpoint from the testdata log's history.
type HistoryFetcher int

func (h *HistoryFetcher) Fetcher() client.Fetcher {
	return func(_ context.Context, p string) ([]byte, error) {
		if p == layout.CheckpointPath {
			p = fmt.Sprintf("%s.%d", layout.CheckpointPath, h)
		}
		path := filepath.Join(testdataDir, "log", p)
		return ioutil.ReadFile(path)
	}

}

// Checkpoint fetches a signed checkpoint for a given historical log size.
func Checkpoint(t *testing.T, size int) []byte {
	t.Helper()
	r, err := ioutil.ReadFile(filepath.Join(testdataDir, "log", fmt.Sprintf("checkpoint.%d", size)))
	if err != nil {
		t.Fatalf("Failed to open checkpoint.%d: %v", size, err)
	}
	return r
}

func locateTestdata() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Dir(filename)
}
