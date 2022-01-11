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
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/serverless/client"
	"github.com/google/trillian-examples/serverless/internal/storage/fs"
	"github.com/google/trillian-examples/serverless/pkg/log"
	"github.com/transparency-dev/merkle"
	"github.com/transparency-dev/merkle/rfc6962"
	"golang.org/x/mod/sumdb/note"

	fmtlog "github.com/google/trillian-examples/formats/log"
)

const (
	pubKey            = "astra+cad5a3d2+AZJqeuyE/GnknsCNh1eCtDtwdAwKBddOlS8M2eI1Jt4b"
	privKey           = "PRIVATE+KEY+astra+cad5a3d2+ASgwwenlc0uuYcdy7kI44pQvuz1fw8cS5NqS8RkZBXoy"
	integrationOrigin = "Serverless Integration Test Log"
)

func RunIntegration(t *testing.T, s log.Storage, f client.Fetcher, lh *rfc6962.Hasher, signer note.Signer) {
	ctx := context.Background()
	lv := merkle.NewLogVerifier(lh)

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

	lst, err := client.NewLogStateTracker(ctx, f, lh, nil, v, integrationOrigin, client.UnilateralConsensus(f))
	if err != nil {
		t.Fatalf("Failed to create new log state tracker: %q", err)
	}

	for i := 0; i < loops; i++ {
		glog.Infof("----------------%d--------------", i)
		checkpoint := lst.LatestConsistent

		// Sequence some leaves:
		leaves := sequenceNLeaves(t, ctx, s, lh, i*leavesPerLoop, leavesPerLoop)

		// Integrate those leaves
		{
			update, err := log.Integrate(ctx, checkpoint, s, lh)
			if err != nil {
				t.Fatalf("Integrate = %v", err)
			}
			update.Origin = integrationOrigin
			cpNote := note.Note{Text: string(update.Marshal())}
			cpNoteSigned, err := note.Sign(&cpNote, signer)
			if err != nil {
				t.Fatalf("Failed to sign Checkpoint: %q", err)
			}
			if err := s.WriteCheckpoint(ctx, cpNoteSigned); err != nil {
				t.Fatalf("Failed to store new log checkpoint: %q", err)
			}
		}

		// State tracker will verify consistency of larger tree
		if _, _, _, err := lst.Update(ctx); err != nil {
			t.Fatalf("Failed to update tracked log state: %q", err)
		}
		newCheckpoint := lst.LatestConsistent
		if got, want := newCheckpoint.Size-checkpoint.Size, uint64(leavesPerLoop); got != want {
			t.Errorf("Integrate missed some entries, got %d want %d", got, want)
		}

		pb, err := client.NewProofBuilder(ctx, newCheckpoint, lh.HashChildren, f)
		if err != nil {
			t.Fatalf("Failed to create ProofBuilder: %q", err)
		}

		for _, l := range leaves {
			h := lh.HashLeaf(l)
			idx, err := client.LookupIndex(ctx, f, h)
			if err != nil {
				t.Fatalf("Failed to lookup leaf index: %v", err)
			}
			ip, err := pb.InclusionProof(ctx, idx)
			if err != nil {
				t.Fatalf("Failed to fetch inclusion proof for %d: %v", idx, err)
			}
			if err := lv.VerifyInclusionProof(int64(idx), int64(newCheckpoint.Size), ip, newCheckpoint.Hash, h); err != nil {
				t.Fatalf("Invalid inclusion proof for %d: %x", idx, ip)
			}
		}
	}
}

func TestServerlessViaFile(t *testing.T) {
	t.Parallel()

	h := rfc6962.DefaultHasher

	// Create log instance
	root := filepath.Join(t.TempDir(), "log")

	// Create signer
	s := mustGetSigner(t, privKey)

	// Create empty checkpoint
	st := mustCreateAndInitialiseStorage(t, context.Background(), root, s)

	// Create file fetcher
	rootURL, err := url.Parse(fmt.Sprintf("file://%s/", root))
	if err != nil {
		t.Fatalf("Failed to create root URL: %q", err)
	}
	f := func(_ context.Context, p string) ([]byte, error) {
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
	t.Parallel()

	h := rfc6962.DefaultHasher

	// Create log instance
	root := filepath.Join(t.TempDir(), "log")

	// Create signer
	s := mustGetSigner(t, privKey)

	// Create empty checkpoint
	st := mustCreateAndInitialiseStorage(t, context.Background(), root, s)

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

func sequenceNLeaves(t *testing.T, ctx context.Context, s log.Storage, lh merkle.LogHasher, start, n int) [][]byte {
	r := make([][]byte, 0, n)
	for i := 0; i < n; i++ {
		c := []byte(fmt.Sprintf("Leaf %d", start+i))
		if _, err := s.Sequence(ctx, lh.HashLeaf(c), c); err != nil {
			t.Fatalf("Sequence = %v", err)
		}
		r = append(r, c)
	}
	return r
}

func httpFetcher(t *testing.T, u string) client.Fetcher {
	t.Helper()
	rootURL, err := url.Parse(u)
	if err != nil {
		t.Fatalf("Failed to create root URL: %q", err)
	}

	return func(ctx context.Context, p string) ([]byte, error) {
		u, err := rootURL.Parse(p)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequest("GET", u.String(), nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req.WithContext(ctx))
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

func mustCreateAndInitialiseStorage(t *testing.T, ctx context.Context, root string, s note.Signer) *fs.Storage {
	t.Helper()
	st, err := fs.Create(root)
	if err != nil {
		t.Fatalf("Create = %v", err)
	}
	cp := fmtlog.Checkpoint{}
	cp.Origin = integrationOrigin
	cpNote := note.Note{Text: string(cp.Marshal())}
	cpNoteSigned, err := note.Sign(&cpNote, s)
	if err != nil {
		t.Fatalf("Failed to sign Checkpoint: %q", err)
	}
	if err := st.WriteCheckpoint(ctx, cpNoteSigned); err != nil {
		t.Fatalf("Failed to store new log checkpoint: %q", err)
	}
	return st
}
