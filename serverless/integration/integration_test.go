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

// Package integration provides an integration test for the serverless example.
package integration

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/serverless/api"
	"github.com/google/trillian-examples/serverless/internal/client"
	"github.com/google/trillian-examples/serverless/internal/log"
	"github.com/google/trillian-examples/serverless/internal/storage/fs"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/merkle/logverifier"
	"github.com/google/trillian/merkle/rfc6962/hasher"
	"golang.org/x/mod/sumdb/note"
)

const (
	pubKey  = "astra+cad5a3d2+AZJqeuyE/GnknsCNh1eCtDtwdAwKBddOlS8M2eI1Jt4b"
	privKey = "PRIVATE+KEY+astra+cad5a3d2+ASgwwenlc0uuYcdy7kI44pQvuz1fw8cS5NqS8RkZBXoy"
)

func RunIntegration(t *testing.T, s log.Storage, f client.FetcherFunc, lh *hasher.Hasher, signer note.Signer) {

	lv := logverifier.New(lh)

	// Do a few interations around the sequence/integrate loop;
	const (
		loops         = 50
		leavesPerLoop = 257
	)

	// Create signature verifier
	v, err := note.NewVerifier(pubKey)
	if err != nil {
		glog.Exitf("Unable to create new verifier: %q", err)
	}

	lst, err := client.NewLogStateTracker(f, lh, nil, v)
	if err != nil {
		t.Fatalf("Failed to create new log state tracker: %q", err)
	}

	for i := 0; i < loops; i++ {
		glog.Infof("----------------%d--------------", i)
		checkpoint := lst.LatestConsistent

		// Sequence some leaves:
		leaves := sequenceNLeaves(t, s, lh, i*leavesPerLoop, leavesPerLoop)

		// Integrate those leaves
		{
			update, err := log.Integrate(s, lh)
			if err != nil {
				t.Fatalf("Integrate = %v", err)
			}
			update.Ecosystem = api.CheckpointHeaderV0
			cpNote := note.Note{Text: string(update.Marshal())}
			cpNoteSigned, err := note.Sign(&cpNote, signer)
			if err != nil {
				t.Fatalf("Failed to sign Checkpoint: %q", err)
			}
			if err := s.WriteCheckpoint(cpNoteSigned); err != nil {
				t.Fatalf("Failed to store new log checkpoint: %q", err)
			}
		}

		// State tracker will verify consistency of larger tree
		if err := lst.Update(); err != nil {
			t.Fatalf("Failed to update tracked log state: %q", err)
		}
		newCheckpoint := lst.LatestConsistent
		if got, want := newCheckpoint.Size-checkpoint.Size, uint64(leavesPerLoop); got != want {
			t.Errorf("Integrate missed some entries, got %d want %d", got, want)
		}

		pb, err := client.NewProofBuilder(newCheckpoint, lh.HashChildren, f)
		if err != nil {
			t.Fatalf("Failed to create ProofBuilder: %q", err)
		}

		for _, l := range leaves {
			h := lh.HashLeaf(l)
			idx, err := client.LookupIndex(f, h)
			if err != nil {
				t.Fatalf("Failed to lookup leaf index: %v", err)
			}
			ip, err := pb.InclusionProof(idx)
			if err != nil {
				t.Fatalf("Failed to fetch inclusion proof for %d: %v", idx, err)
			}
			if err := lv.VerifyInclusionProof(int64(idx), int64(newCheckpoint.Size), ip, newCheckpoint.RootHash, h); err != nil {
				t.Fatalf("Invalid inclusion proof for %d: %x", idx, ip)
			}
		}
	}
}

func TestServerlessViaFile(t *testing.T) {
	h := hasher.DefaultHasher

	// Create log instance
	root := filepath.Join(t.TempDir(), "log")

	// Create signer
	s := mustGetSigner(t, privKey)

	// Create empty checkpoint
	st := mustCreateAndInitialiseStorage(t, root, h, s)

	// Create file fetcher
	rootURL, err := url.Parse(fmt.Sprintf("file://%s/", root))
	if err != nil {
		t.Fatalf("Failed to create root URL: %q", err)
	}
	f := func(p string) ([]byte, error) {
		u, err := rootURL.Parse(p)
		if err != nil {
			return nil, err
		}
		return ioutil.ReadFile(u.Path)
	}

	// Run test
	RunIntegration(t, st, f, h, s)
}

func TestServerlessViaHTTP(t *testing.T) {
	h := hasher.DefaultHasher

	// Create log instance
	root := filepath.Join(t.TempDir(), "log")

	// Create signer
	s := mustGetSigner(t, privKey)

	// Create empty checkpoint
	st := mustCreateAndInitialiseStorage(t, root, h, s)

	// Arrange for its files to be served via HTTP
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to create listener: %q", err)
	}
	srv := http.Server{
		Handler: http.FileServer(http.Dir(root)),
	}
	defer srv.Close()
	go func() {
		srv.Serve(listener)
	}()

	// Create fetcher
	url := fmt.Sprintf("http://%s/", listener.Addr().String())
	f := httpFetcher(t, url)

	// Run test
	RunIntegration(t, st, f, h, s)
}

func sequenceNLeaves(t *testing.T, s log.Storage, lh hashers.LogHasher, start, n int) [][]byte {
	r := make([][]byte, 0, n)
	for i := 0; i < n; i++ {
		c := []byte(fmt.Sprintf("Leaf %d", start+i))
		if _, err := s.Sequence(lh.HashLeaf(c), c); err != nil {
			t.Fatalf("Sequence = %v", err)
		}
		r = append(r, c)
	}
	return r
}

func httpFetcher(t *testing.T, u string) client.FetcherFunc {
	t.Helper()
	rootURL, err := url.Parse(u)
	if err != nil {
		t.Fatalf("Failed to create root URL: %q", err)
	}

	return func(p string) ([]byte, error) {
		u, err := rootURL.Parse(p)
		if err != nil {
			return nil, err
		}
		resp, err := http.Get(u.String())
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		return ioutil.ReadAll(resp.Body)
	}
}

func mustGetSigner(t *testing.T, privKey string) note.Signer {
	t.Helper()
	s, err := note.NewSigner(privKey)
	if err != nil {
		glog.Exitf("Failed to instantiate signer: %q", err)
	}
	return s
}

func mustCreateAndInitialiseStorage(t *testing.T, root string, h *hasher.Hasher, s note.Signer) *fs.Storage {
	t.Helper()
	st, err := fs.Create(root, h.EmptyRoot())
	if err != nil {
		t.Fatalf("Create = %v", err)
	}
	cp := st.Checkpoint()
	cp.Ecosystem = api.CheckpointHeaderV0
	cpNote := note.Note{Text: string(cp.Marshal())}
	cpNoteSigned, err := note.Sign(&cpNote, s)
	if err != nil {
		t.Fatalf("Failed to sign Checkpoint: %q", err)
	}
	if err := st.WriteCheckpoint(cpNoteSigned); err != nil {
		t.Fatalf("Failed to store new log checkpoint: %q", err)
	}
	return st
}
