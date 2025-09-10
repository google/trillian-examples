// Copyright 2025 Google LLC. All Rights Reserved.
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

package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/transparency-dev/incubator/vindex/api"
	"k8s.io/klog/v2"
)

func NewServer(lookup func(context.Context, [sha256.Size]byte) (api.LookupResponse, error)) Server {
	return Server{
		lookup: lookup,
	}
}

type Server struct {
	lookup func(context.Context, [sha256.Size]byte) (api.LookupResponse, error)
}

// handleLookup handles GET requests for looking up map entries.
func (s Server) handleLookup(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hashStr, ok := vars["hash"]
	if !ok {
		http.Error(w, "hash parameter not found", http.StatusBadRequest)
		return
	}

	h, err := hex.DecodeString(hashStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid hex hash: %v", err), http.StatusBadRequest)
		return
	}
	if l := len(h); l != sha256.Size {
		http.Error(w, fmt.Sprintf("hash wrong length (decoded %d bytes)", l), http.StatusBadRequest)
		return
	}

	klog.V(2).Infof("Received hash from request: '%s'", h)

	resp, err := s.lookup(r.Context(), [sha256.Size]byte(h))
	if err != nil {
		http.Error(w, fmt.Sprintf("lookup failed: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		klog.Warningf("failed to encode response: %v", err)
	}
}

func (s Server) registerHandlers(r *mux.Router) {
	r.HandleFunc("/vindex/lookup/{hash}", s.handleLookup).Methods("GET")
}
