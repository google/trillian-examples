// Copyright 2023 Google LLC. All Rights Reserved.
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

// Package http contains private implementation details for the distributor server.
package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/distributor/api"
	"github.com/google/trillian-examples/distributor/cmd/internal/distributor"
	"github.com/gorilla/mux"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server is the core handler implementation of the witness.
type Server struct {
	d *distributor.Distributor
}

// NewServer creates a new server.
func NewServer(d *distributor.Distributor) *Server {
	return &Server{
		d: d,
	}
}

// update handles requests to update checkpoints.
func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	v := mux.Vars(r)
	logID := v["logid"]
	witID := v["witid"]
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("cannot read request body: %v", err.Error()), http.StatusBadRequest)
		return
	}
	if err := s.d.Distribute(r.Context(), logID, witID, body); err != nil {
		glog.Warningf("failed to update to new checkpoint: %v", err)
		http.Error(w, "failed to update to new checkpoint", httpForCode(status.Code(err)))
		return
	}
}

// getCheckpointN returns a checkpoint stored for a given log with the specified number of witnesses.
func (s *Server) getCheckpointN(w http.ResponseWriter, r *http.Request) {
	v := mux.Vars(r)
	logID := v["logid"]
	numSigs, err := strconv.ParseUint(v["numsigs"], 10, 32)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to parse number of signatures: %v", err), http.StatusBadRequest)
		return
	}
	// Get the signed checkpoint from the witness.
	chkpt, err := s.d.GetCheckpointN(r.Context(), logID, uint32(numSigs))
	if err != nil {
		glog.Warningf("failed to get checkpoint: %v", err)
		http.Error(w, "failed to get checkpoint", httpForCode(status.Code(err)))
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write(chkpt)
}

// getCheckpointWitness returns the latest checkpoint stored for a given log by the given witness.
func (s *Server) getCheckpointWitness(w http.ResponseWriter, r *http.Request) {
	v := mux.Vars(r)
	logID := v["logid"]
	witID := v["witid"]

	// Get the signed checkpoint from the witness.
	chkpt, err := s.d.GetCheckpointWitness(r.Context(), logID, witID)
	if err != nil {
		glog.Warningf("failed to get checkpoint: %v", err)
		http.Error(w, "failed to get checkpoint", httpForCode(status.Code(err)))
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write(chkpt)
}

// getLogs returns a list of all logs the witness is aware of.
func (s *Server) getLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := s.d.GetLogs(r.Context())
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
	witStr := "{witid:[a-zA-Z0-9-\\.]+}"
	r.HandleFunc(fmt.Sprintf(api.HTTPGetCheckpointN, logStr, "{numsigs:\\d+}"), s.getCheckpointN).Methods("GET")
	r.HandleFunc(fmt.Sprintf(api.HTTPCheckpointByWitness, logStr, witStr), s.update).Methods("PUT")
	r.HandleFunc(fmt.Sprintf(api.HTTPCheckpointByWitness, logStr, witStr), s.getCheckpointWitness).Methods("GET")
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
