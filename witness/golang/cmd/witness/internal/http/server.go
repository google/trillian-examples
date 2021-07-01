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

// Package http contains private implementation details for the witness server.
package http

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/google/trillian-examples/witness/golang/api"
	"github.com/google/trillian-examples/witness/golang/cmd/witness/internal/witness"
	"github.com/gorilla/mux"
)

// Server is the core handler implementation of the witness.
type Server struct {
	w *witness.Witness
}

// NewServer creates a new server.
func NewServer(witness *witness.Witness) *Server {
	return &Server{
		w: witness,
	}
}

// update handles requests to update checkpoints.
// It expects a POSTed body containing a JSON-formatted api.UpdateRequest
// statement.
func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	h := r.Header["Content-Type"]
	if len(h) == 0 {
		http.Error(w, "need a content header", http.StatusBadRequest)
	}
	if h[0] != "application/json" {
		http.Error(w, "need request in JSON format", http.StatusBadRequest)
		return
	}
	v := mux.Vars(r)
	logID := v["logid"]
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("cannot read request body: %v", err.Error()), http.StatusBadRequest)
		return
	}
	var req api.UpdateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, fmt.Sprintf("cannot parse request body as proper JSON struct: %v", err.Error()), http.StatusBadRequest)
		return
	}
	// Get the checkpoint size from the witness.
	size, err := s.w.Update(r.Context(), logID, req.Checkpoint, req.Proof)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to update to new checkpoint: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(strconv.FormatUint(size, 10)))
}

// getCheckpoint returns a checkpoint stored for a given log.
func (s *Server) getCheckpoint(w http.ResponseWriter, r *http.Request) {
	v := mux.Vars(r)
	logID := v["logid"]
	// Get the signed checkpoint from the witness.
	chkpt, err := s.w.GetCheckpoint(logID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get checkpoint: %q", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write(chkpt)
}

// RegisterHandlers registers HTTP handlers for witness endpoints.
func (s *Server) RegisterHandlers(r *mux.Router) {
	r.HandleFunc(fmt.Sprintf("/%s/{logid}", api.HTTPGetCheckpoint), s.getCheckpoint).Methods("GET")
	r.HandleFunc(fmt.Sprintf("/%s/{logid}", api.HTTPUpdate), s.update).Methods("POST")
}
