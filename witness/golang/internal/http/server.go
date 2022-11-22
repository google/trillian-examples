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
	"io"
	"net/http"

	"github.com/google/trillian-examples/witness/golang/api"
	"github.com/google/trillian-examples/witness/golang/internal/witness"
	"github.com/gorilla/mux"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
// statement and returns a JSON-formatted api.UpdateResponse statement.
func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	v := mux.Vars(r)
	logID := v["logid"]
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("cannot read request body: %v", err.Error()), http.StatusBadRequest)
		return
	}
	var req api.UpdateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, fmt.Sprintf("cannot parse request body as proper JSON struct: %v", err.Error()), http.StatusBadRequest)
		return
	}
	// Get the output from the witness.
	chkpt, err := s.w.Update(r.Context(), logID, req.Checkpoint, req.Proof)
	if err != nil {
		c := status.Code(err)
		// If there was an AlreadyExists it's possible the caller was
		// just out of date.  Give the returned checkpoint to help them
		// form a new request.
		if c == codes.AlreadyExists {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.WriteHeader(httpForCode(c))
			// The checkpoint body will be written below...
		} else {
			http.Error(w, fmt.Sprintf("failed to update to new checkpoint: %v", err), httpForCode(c))
			return
		}
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write(chkpt)
}

// getCheckpoint returns a checkpoint stored for a given log.
func (s *Server) getCheckpoint(w http.ResponseWriter, r *http.Request) {
	v := mux.Vars(r)
	logID := v["logid"]
	// Get the signed checkpoint from the witness.
	chkpt, err := s.w.GetCheckpoint(logID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get checkpoint: %v", err), httpForCode(status.Code(err)))
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write(chkpt)
}

// getLogs returns a list of all logs the witness is aware of.
func (s *Server) getLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := s.w.GetLogs()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get log list: %v", err), http.StatusInternalServerError)
		return
	}
	logList, err := json.Marshal(logs)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to convert log list to JSON: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/json")
	w.Write(logList)
}

// RegisterHandlers registers HTTP handlers for witness endpoints.
func (s *Server) RegisterHandlers(r *mux.Router) {
	logStr := "{logid:[a-zA-Z0-9-]+}"
	r.HandleFunc(fmt.Sprintf(api.HTTPGetCheckpoint, logStr), s.getCheckpoint).Methods("GET")
	r.HandleFunc(fmt.Sprintf(api.HTTPUpdate, logStr), s.update).Methods("PUT")
	r.HandleFunc(api.HTTPGetLogs, s.getLogs).Methods("GET")
}

func httpForCode(c codes.Code) int {
	switch c {
	case codes.AlreadyExists:
		return http.StatusConflict
	case codes.NotFound:
		return http.StatusNotFound
	case codes.FailedPrecondition, codes.InvalidArgument:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
