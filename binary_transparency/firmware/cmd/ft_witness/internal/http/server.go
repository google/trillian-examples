// Copyright 2020 Google LLC. All Rights Reserved.
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

// Package http contains private implementation details for the FirmwareTransparency witness server.
package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/client"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/verify"
	"github.com/gorilla/mux"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// WS is the interface to the  Witness Store, for storage of latest checkpoint
type WS interface {
	// Store puts the checkpoint into Witness Store
	StoreCP(wcp api.LogCheckpoint) error

	// Retrieve gets the stored checkpoint.
	// Must return status code NotFound if no such checkpoint exists.
	RetrieveCP() (api.LogCheckpoint, error)
}

// Server is the core state & handler implementation of the FT Witness
type Server struct {
	ws WS
}

// NewServer creates a new server.
func NewServer(ws WS) *Server {
	return &Server{
		ws: ws,
	}
}

// getWcheckpoint returns a checkpoint which is registered with witness
func (s *Server) getWcheckpoint(w http.ResponseWriter, r *http.Request) {
	checkpoint, err := s.ws.RetrieveCP()
	if err != nil {
		http.Error(w, err.Error(), httpStatusForErr(err))
		return
	}

	js, err := json.Marshal(checkpoint)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

// httpStatusForErr maps status codes to HTTP errors.
func httpStatusForErr(e error) int {
	switch status.Code(e) {
	case codes.OK:
		return http.StatusOK
	case codes.NotFound:
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
	// unreachable
}

// RegisterHandlers registers HTTP handlers for firmware transparency endpoints.
func (s *Server) RegisterHandlers(r *mux.Router) {
	r.HandleFunc(fmt.Sprintf("/%s", api.WitnessGetCheckpoint), s.getWcheckpoint).Methods("GET")
}

// PollFTLog periodically polls the FT log for updating the witness checkpoint.
func (s *Server) PollFTLog(ctx context.Context, LogURL string, PollInterval time.Duration) error {

	ticker := time.NewTicker(PollInterval)
	ftURL, err := url.Parse(LogURL)
	if err != nil {
		return fmt.Errorf("failed to parse FT log URL: %w", err)
	}
	glog.Infof("Polling FT log %q...", ftURL)
	c := client.ReadonlyClient{LogURL: ftURL}
	lv := verify.NewLogVerifier()
	for {
		wcp, err := s.ws.RetrieveCP()

		select {
		case <-ticker.C:
			//
		case <-ctx.Done():
			return ctx.Err()
		}
		cp, err := c.GetCheckpoint()
		if err != nil {
			glog.Warningf("Failed to update LogCheckpoint: %q", err)
			continue
		}
		if cp.TreeSize <= wcp.TreeSize {
			continue
		}
		glog.V(1).Infof("Got newer checkpoint %s", cp)
		// Perform consistency check only for non-zero saved witness tree size
		if wcp.TreeSize != 0 {
			consistency, err := c.GetConsistencyProof(api.GetConsistencyRequest{From: wcp.TreeSize, To: cp.TreeSize})
			if err != nil {
				glog.Warningf("Failed to fetch the Consistency: %q", err)
				continue
			}
			glog.V(1).Infof("Printing the latest Consistency Proof Information")
			glog.V(1).Infof("Consistency Proof = %x", consistency.Proof)

			//Verify the fetched consistency proof
			if err := lv.VerifyConsistencyProof(int64(wcp.TreeSize), int64(cp.TreeSize), wcp.RootHash, cp.RootHash, consistency.Proof); err != nil {
				// Verification of Consistency Proof failed!!
				glog.Warningf("Failed verification of Consistency proof %q", err)
				continue
			}
			glog.V(1).Infof("Consistency proof for Treesize %d verified", cp.TreeSize)
		}
		wcp = *cp
		//TO DO Check for Errors here!
		s.ws.StoreCP(wcp)
	}
}
