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

// Package p provides Google Cloud Function for sequencing entries in a
// serverless log.
package p

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/transparency-dev/merkle/rfc6962"
	"golang.org/x/mod/sumdb/note"

	"github.com/google/trillian-examples/serverless/pkg/log"
	"github.com/gcp_serverless_module/internal/storage"

	fmtlog "github.com/google/trillian-examples/formats/log"
	golog "log"
	stErrors "github.com/google/trillian-examples/serverless/pkg/storage/errors"
)

func validateCommonArgs(w http.ResponseWriter, origin string) (ok bool) {
	if len(origin) == 0 {
		http.Error(w, "Please set `origin` in HTTP body to log identifier.", http.StatusBadRequest)
		return false
	}

	pubKey := os.Getenv("SERVERLESS_LOG_PUBLIC_KEY")
	if len(pubKey) == 0 {
		http.Error(w,
			"Please set SERVERLESS_LOG_PUBLIC_KEY environment variable",
			http.StatusBadRequest)
		return false
	}

	return true
}

// Sequence is the entrypoint of the `sequence` GCF function.
func Sequence(w http.ResponseWriter, r *http.Request) {
	var d struct {
		Origin     string `json:"origin"`
		Bucket 		 string `json:"bucket"`
		Entries 	 string `prefix:"entries"`
	}

	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		golog.Printf("json.NewDecoder: %v", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	if ok := validateCommonArgs(w, d.Origin); !ok {
		return
	}

	// TODO(jayhou): list entries objects
	it := bkt.Objects(ctx, &gcs.Query{
		Prefix: tPath, // TODO(jayhou): get path under the pending dir?
	})

	h := rfc6962.DefaultHasher
	// init storage

	ctx := context.Background()
	client, err := storage.NewClient(ctx, os.Getenv("GCP_PROJECT"))
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create GCS client: %v", err), http.StatusBadRequest)
		return
	}

	// TODO(jayhou): should this take a bucket name param?
	cpRaw, err := client.ReadCheckpoint(d.Bucket)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read log checkpoint: %q", err), http.StatusBadRequest)
		return
	}

	// Check signatures
	v, err := note.NewVerifier(pubKey)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to instantiate Verifier: %q", err), http.StatusBadRequest)
		return
	}
	cp, _, _, err := fmtlog.ParseCheckpoint(cpRaw, d.Origin, v)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse Checkpoint: %q", err), http.StatusBadRequest)
		return
	}

	// sequence entries

	// entryInfo binds the actual bytes to be added as a leaf with a
	// user-recognisable name for the source of those bytes.
	// The name is only used below in order to inform the user of the
	// sequence numbers assigned to the data from the provided input files.
	type entryInfo struct {
		name string
		b    []byte
	}
	entries := make(chan entryInfo, 100)
	go func() {
		for {
			attrs, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				// TODO(jayhou): log this?
				fmt.Errorf("failed to get object '%s' from bucket '%s': %v", tPath, c.bucket, err)
			}

			r, err := bkt.Object(attrs.Name).NewReader(ctx)
			if err != nil {
				// TODO(jayhou): log this?
				fmt.Errorf("failed to create reader for object '%s' in bucket '%s': %v", attrs.Name, c.bucket, err)
			}
			defer r.Close()

			b, err := ioutil.ReadAll(r)
			if err != nil {
				// TODO(jayhou): log failure
			}

			entries <- entryInfo{name: attrs.Name, b: b}
		}
		close(entries)
	}()

	for entry := range entries {
		// ask storage to sequence
		lh := h.HashLeaf(entry.b)
		dupe := false
		seq, err := client.Sequence(lh, entry.b)
		if err != nil {
			if errors.Is(err, stErrors.ErrDupeLeaf) {
				dupe = true
			} else {
				// TODO(jayhou): log this
				glog.Exitf("failed to sequence %q: %q", entry.name, err)
			}
		}
		l := fmt.Sprintf("%d: %v", seq, entry.name)
		if dupe {
			l += " (dupe)"
		}
		// TODO(jayhou): log this
		glog.Info(l)
	}
}

// Integrate is the entrypoint of the `integrate` GCF function.
func Integrate(w http.ResponseWriter, r *http.Request) {
	var d struct {
		Origin     string `json:"origin"`
		Initialise bool   `json:"initialise"`
		Bucket string `json:"bucket"`
	}

	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		golog.Printf("json.NewDecoder: %v", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	if ok := validateCommonArgs(w, d.Origin); !ok {
		return
	}

	privKey := os.Getenv("SERVERLESS_LOG_PRIVATE_KEY")
	if len(privKey) == 0 {
		http.Error(w,
			"Please set SERVERLESS_LOG_PUBLIC_KEY environment variable",
			http.StatusBadRequest)
	}

	s, err := note.NewSigner(privKey)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to instantiate signer: %q", err), http.StatusInternalServerError)
		return
	}

	ctx := context.Background()
	client, err := storage.NewClient(ctx, os.Getenv("GCP_PROJECT"))
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create GCS client: %v", err), http.StatusBadRequest)
		return
	}

	var cpNote note.Note
	h := rfc6962.DefaultHasher
	if d.Initialise {
		if err := client.Create(ctx, d.Bucket); err != nil {
			http.Error(w, fmt.Sprintf("Failed to create bucket for log: %v", err), http.StatusBadRequest)
			return
		}

		cp := fmtlog.Checkpoint{
			Hash: h.EmptyRoot(),
		}
		if err := signAndWrite(ctx, &cp, cpNote, s, client, d.Origin); err != nil {
			http.Error(w, fmt.Sprintf("Failed to sign: %q", err), http.StatusInternalServerError)
		}
		fmt.Fprintf(w, fmt.Sprintf("Initialised log at %s.", d.Bucket))
		return
	}

	// init storage
	cpRaw, err := client.ReadCheckpoint(ctx)
	if err != nil {
		http.Error(w,
			fmt.Sprintf("Failed to read log checkpoint: %q", err),
			http.StatusInternalServerError)
	}

	// Check signatures
	v, err := note.NewVerifier(pubKey)
	if err != nil {
		http.Error(w,
			fmt.Sprintf("Failed to instantiate Verifier: %q", err),
			http.StatusInternalServerError)
	}
	cp, _, _, err := fmtlog.ParseCheckpoint(cpRaw, d.Origin, v)
	if err != nil {
		http.Error(w,
			fmt.Sprintf("Failed to open Checkpoint: %q", err),
			http.StatusInternalServerError)
	}

	// Integrate new entries
	newCp, err := log.Integrate(ctx, *cp, client, h)
	if err != nil {
		http.Error(w,
			fmt.Sprintf("Failed to integrate: %q", err),
			http.StatusInternalServerError)
	}
	if newCp == nil {
		http.Error(w, "Nothing to integrate", http.StatusInternalServerError)
	}

	err = signAndWrite(ctx, newCp, cpNote, s, client, d.Origin)
	if err != nil {
		http.Error(w,
			fmt.Sprintf("Failed to sign: %q", err),
			http.StatusInternalServerError)
	}

	return
}

func signAndWrite(ctx context.Context, cp *fmtlog.Checkpoint, cpNote note.Note, s note.Signer, client *storage.Client, origin string) error {
	cp.Origin = origin
	cpNote.Text = string(cp.Marshal())
	cpNoteSigned, err := note.Sign(&cpNote, s)
	if err != nil {
		return fmt.Errorf("failed to sign Checkpoint: %w", err)
	}
	if err := client.WriteCheckpoint(ctx, cpNoteSigned); err != nil {
		return fmt.Errorf("failed to store new log checkpoint: %w", err)
	}
	return nil
}
