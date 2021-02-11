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

// Package http contains private implementation details for the FirmwareTransparency witness.
package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/client"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/verify"
	"github.com/gorilla/mux"
)

// WitnessStore is the interface to the  Witness Store, for storage of latest checkpoint
type WitnessStore interface {
	// Store puts the checkpoint into Witness Store
	StoreCP(wcp api.LogCheckpoint) error

	// Retrieve gets the stored checkpoint.
	RetrieveCP() (api.LogCheckpoint, error)
}

// Witness is the core state & handler implementation of the FT Witness
type Witness struct {
	ws           WitnessStore
	gcp          api.LogCheckpoint
	logURL       string
	pollInterval time.Duration
	witnessLock  sync.Mutex
}

// NewWitness creates a new Witness.
func NewWitness(ws WitnessStore, logURL string, pollInterval time.Duration) (*Witness, error) {
	gcp, err := ws.RetrieveCP()
	if err != nil {
		return nil, fmt.Errorf("new witness failed due to storage retrieval: %w", err)
	}
	return &Witness{
		ws:           ws,
		gcp:          gcp,
		logURL:       logURL,
		pollInterval: pollInterval,
	}, nil
}

// getCheckpoint returns a checkpoint which is registered with witness
func (s *Witness) getCheckpoint(w http.ResponseWriter, r *http.Request) {
	s.witnessLock.Lock()
	js, err := json.Marshal(s.gcp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	s.witnessLock.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

// RegisterHandlers registers HTTP handlers for firmware transparency endpoints.
func (s *Witness) RegisterHandlers(r *mux.Router) {
	r.HandleFunc(fmt.Sprintf("/%s", api.WitnessGetCheckpoint), s.getCheckpoint).Methods("GET")
}

// Poll periodically polls the FT log for updating the witness checkpoint.
// It only returns on error (when it doesn't start its own polling thread)
func (s *Witness) Poll(ctx context.Context) error {
	ticker := time.NewTicker(s.pollInterval)
	ftURL, err := url.Parse(s.logURL)
	if err != nil {
		return fmt.Errorf("failed to parse FT log URL: %w", err)
	}
	glog.Infof("Polling FT log %q...", ftURL)
	c := client.ReadonlyClient{LogURL: ftURL}
	lv := verify.NewLogVerifier()
	wcp := s.gcp
	for {

		select {
		case <-ticker.C:
			//
		case <-ctx.Done():
			return ctx.Err()
		}
		cp, err := c.GetCheckpoint()
		if err != nil {
			glog.Warningf("Failed to get logcheckpoint: %q", err)
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
			glog.V(2).Infof("Printing the latest Consistency Proof Information")
			glog.V(2).Infof("Consistency Proof = %x", consistency.Proof)

			//Verify the fetched consistency proof
			if err := lv.VerifyConsistencyProof(int64(wcp.TreeSize), int64(cp.TreeSize), wcp.RootHash, cp.RootHash, consistency.Proof); err != nil {
				// Verification of Consistency Proof failed!!
				glog.Warningf("Failed verification of Consistency proof %q", err)
				continue
			}
			glog.V(1).Infof("Consistency proof for Treesize %d verified", cp.TreeSize)
		}
		s.witnessLock.Lock()
		if err = s.ws.StoreCP(*cp); err != nil {
			glog.Warningf("Failed to save new logcheckpoint into store: %q", err)
			s.witnessLock.Unlock()
			continue
		}
		s.gcp = (*cp)
		s.witnessLock.Unlock()
	}
}
