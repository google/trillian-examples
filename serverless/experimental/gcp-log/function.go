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
)

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

	if len(d.Origin) == 0 {
		http.Error(w, "Please set `origin` in HTTP body to log identifier.", http.StatusBadRequest)
		return
	}

	pubKey := os.Getenv("SERVERLESS_LOG_PUBLIC_KEY")
	if len(pubKey) == 0 {
		http.Error(w,
			"Please set SERVERLESS_LOG_PUBLIC_KEY environment variable",
			http.StatusBadRequest)
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
