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

// Package p provides Google Cloud Functions for adding (sequencing and
// integrating) new entries to a serverless log.
package p

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/transparency-dev/merkle/rfc6962"
	"golang.org/x/mod/sumdb/note"
	"google.golang.org/api/iterator"

	"github.com/gcp_serverless_module/internal/storage"
	"github.com/google/trillian-examples/serverless/pkg/log"

	fmtlog "github.com/google/trillian-examples/formats/log"
)

func validateCommonArgs(w http.ResponseWriter, origin string) (ok bool, pubKey string) {
	if len(origin) == 0 {
		http.Error(w, "Please set `origin` in HTTP body to log identifier.", http.StatusBadRequest)
		return false, ""
	}

	pubKey = os.Getenv("SERVERLESS_LOG_PUBLIC_KEY")
	if len(pubKey) == 0 {
		http.Error(w,
			"Please set SERVERLESS_LOG_PUBLIC_KEY environment variable",
			http.StatusBadRequest)
		return false, ""
	}

	return true, pubKey
}

// Sequence is the entrypoint of the `sequence` GCF function.
func Sequence(w http.ResponseWriter, r *http.Request) {
	// TODO(jayhou): validate that EntriesDir is only touching the log path.

	var d struct {
		Bucket     string `json:"bucket"`
		EntriesDir string `json:"entriesDir"`
		Origin     string `json:"origin"`
	}

	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		code := http.StatusBadRequest
		fmt.Printf("json.NewDecoder: %v", err)
		http.Error(w, http.StatusText(code), code)
		return
	}

	ok, pubKey := validateCommonArgs(w, d.Origin)
	if !ok {
		return
	}

	// init storage

	ctx := context.Background()
	client, err := storage.NewClient(ctx, os.Getenv("GCP_PROJECT"), d.Bucket)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create GCS client: %q", err), http.StatusInternalServerError)
		return
	}

	// Read the current log checkpoint to retrieve next sequence number.

	cpRaw, err := client.ReadCheckpoint(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read log checkpoint: %q", err), http.StatusInternalServerError)
		return
	}

	// Check signatures
	v, err := note.NewVerifier(pubKey)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to instantiate Verifier: %q", err), http.StatusInternalServerError)
		return
	}
	cp, _, _, err := fmtlog.ParseCheckpoint(cpRaw, d.Origin, v)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse Checkpoint: %q", err), http.StatusInternalServerError)
		return
	}
	client.SetNextSeq(cp.Size)

	// sequence entries

	h := rfc6962.DefaultHasher
	it := client.GetObjects(ctx, d.EntriesDir)
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			http.Error(w,
				fmt.Sprintf("Bucket(%q).Objects: %v", d.Bucket, err),
				http.StatusInternalServerError)
			return
		}
		// Skip this directory - only add files under it.
		if attrs.Name == d.EntriesDir {
			continue
		}

		bytes, err := client.GetObjectData(ctx, attrs.Name)
		if err != nil {
			http.Error(w,
				fmt.Sprintf("Failed to get data of object %q: %q", attrs.Name, err),
				http.StatusInternalServerError)
			return
		}

		// ask storage to sequence
		lh := h.HashLeaf(bytes)
		dupe := false
		seq, err := client.Sequence(ctx, lh, bytes)
		if err != nil {
			if errors.Is(err, log.ErrDupeLeaf) {
				dupe = true
			} else {
				http.Error(w,
					fmt.Sprintf("Failed to sequence %q: %q", attrs.Name, err),
					http.StatusInternalServerError)
				return
			}

			l := fmt.Sprintf("Sequence num %d assigned to %s", seq, attrs.Name)
			if dupe {
				l += " (dupe)"
			}
			fmt.Println(l)
		}
	}
}

// Integrate is the entrypoint of the `integrate` GCF function.
func Integrate(w http.ResponseWriter, r *http.Request) {
	var d struct {
		Origin     string `json:"origin"`
		Initialise bool   `json:"initialise"`
		Bucket     string `json:"bucket"`
	}

	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		fmt.Printf("json.NewDecoder: %v", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	ok, pubKey := validateCommonArgs(w, d.Origin)
	if !ok {
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
	client, err := storage.NewClient(ctx, os.Getenv("GCP_PROJECT"), d.Bucket)
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
		return
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
