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
	"github.com/google/trillian-examples/witness/golang/cmd/witness/internal/witness"
)

// Server is the core handler implementation of the witness.
type Server struct {
	w      Witness
	ctx    context.Context
}

// NewServer creates a new server.
func NewServer(ctx context.Context, witness Witness) *Server {
	return &Server{
		w:      witness,
		ctx:	ctx,
	}
}

func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	logID, chkpt, pf, err := parseUpdateRequest(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to parse request: %q", err.Error()), http.StatusBadRequest)
		return
	}
	// Get the checkpoint size from the witness.
	size, err := s.w.Update(s.ctx, logID, chkpt, pf)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to update to new checkpoint: %q", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
}

func (s *Server) getCheckpoint(w http.ResponseWriter, r *http.Request) {
	logID, err := parseStringParam(r, "logID")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// then same thing, we run the witness function and return via w
}

// parseUpdateRequest returns the logID string, bytes for the checkpoint, and
// byte array for the proof.
func parseUpdateRequest(r *http.Request) (string, []byte, [][]byte, error) {
	h := r.Header["Content-Type"]
	if len(h) == 0 {
		return nil, nil, fmt.Errorf("no content-type header")
	}

	mediaType, mediaParams, err := mime.ParseMediaType(h[0])
	if err != nil {
		return nil, nil, err
	}
	if !strings.HasPrefix(mediaType, "multipart/") {
		return nil, nil, fmt.Errorf("expecting mime multipart body")
	}
	boundary := mediaParams["boundary"]
	if len(boundary) == 0 {
		return nil, nil, fmt.Errorf("invalid mime multipart header - no boundary specified")
	}
	mr := multipart.NewReader(r.Body, boundary)

	// Get firmware statement (JSON)
	p, err := mr.NextPart()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find firmware statement in request body: %v", err)
	}
	rawJSON, err := ioutil.ReadAll(p)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read body of firmware statement: %v", err)
	}

	// Get firmware binary image
	p, err = mr.NextPart()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find firmware image in request body: %v", err)
	}
	image, err := ioutil.ReadAll(p)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read body of firmware image: %v", err)
	}
	return rawJSON, image, nil
}

// RegisterHandlers registers HTTP handlers for witness endpoints.
func (s *Server) RegisterHandlers(r *mux.Router) {
	r.HandleFunc(fmt.Sprintf("/%s", api.HTTPGetCheckpoint), s.getCheckpoint).Methods("GET")
	r.HandleFunc(fmt.Sprintf("/%s", api.HTTPUpdate), s.update).Methods("POST")
}
