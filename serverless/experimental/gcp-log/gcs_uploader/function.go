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
//
// Package p provides a Google Cloud Function to write an object to GCS.
package p

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	gcs "cloud.google.com/go/storage"
)

// CreateGCSObject writes a GCS object with `entryContent` to `entryPath` in
// `bucket`.
func CreateGCSObject(w http.ResponseWriter, r *http.Request) {
	var d struct {
		EntryContent string `json:"entryContent"`
		Bucket       string `json:"bucket"`
		EntryPath    string `json:"entryPath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		code := http.StatusBadRequest
		fmt.Printf("json.NewDecoder: %v", err)
		http.Error(w, http.StatusText(code), code)
		return
	}
	if d.EntryContent == "" {
		http.Error(w, "entryContent must not be empty",
			http.StatusBadRequest)
		return
	}
	if d.Bucket == "" {
		http.Error(w, "bucket must not be empty",
			http.StatusBadRequest)
		return
	}
	if d.EntryPath == "" {
		http.Error(w, "entryPath must not be empty",
			http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	client, err := gcs.NewClient(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create GCS client: %q", err),
			http.StatusInternalServerError)
		return
	}

	obj := client.Bucket(d.Bucket).Object(d.EntryPath)
	writer := obj.NewWriter(ctx)
	if n, err := writer.Write([]byte(d.EntryContent)); err != nil {
		http.Error(w,
			fmt.Sprintf("Failed to write GCS obj %q: %q", d.EntryPath, err),
			http.StatusInternalServerError)
		return
	} else if n != len(d.EntryContent) {
		http.Error(w,
			fmt.Sprintf("Failed to write GCS obj %q: only wrote %d of %d bytes", d.EntryPath, n, len(d.EntryContent)),
			http.StatusInternalServerError)
		return
	}

	if err := writer.Close(); err != nil {
		http.Error(w,
			fmt.Sprintf("Failed to close GCS obj %q: %q", d.EntryPath, err),
			http.StatusInternalServerError)
	}

	fmt.Fprintf(w, "Successfully wrote GCS obj %q", d.EntryPath)
}
