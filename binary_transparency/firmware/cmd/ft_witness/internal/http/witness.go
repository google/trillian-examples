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
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/client"
	"github.com/gorilla/mux"
	"golang.org/x/mod/sumdb/note"
)

// WitnessStore is the interface to the  Witness Store, for storage of latest checkpoint
type WitnessStore interface {
	// Store puts the checkpoint into Witness Store
	StoreCP([]byte) error

	// Retrieve gets the stored checkpoint.
	RetrieveCP() ([]byte, error)
}

// Witness is the core state & handler implementation of the FT Witness
type Witness struct {
	ws             WitnessStore
	gcp            api.LogCheckpoint
	logURL         string
	logSigVerifier note.Verifier
	pollInterval   time.Duration
	witnessLock    sync.Mutex
}

// NewWitness creates a new Witness.
func NewWitness(ws WitnessStore, logURL string, logSigVerifier note.Verifier, pollInterval time.Duration) (*Witness, error) {
	gcpRaw, err := ws.RetrieveCP()
	if err != nil {
		return nil, fmt.Errorf("new witness failed due to storage retrieval: %w", err)
	}
	gcp := api.LogCheckpoint{
		Envelope: gcpRaw,
	}
	if len(gcpRaw) > 0 {
		cp, err := api.ParseCheckpoint(gcpRaw, logSigVerifier)
		if err != nil {
			return nil, fmt.Errorf("failed to open stored checkpoint: %w", err)
		}
		gcp = *cp
	}

	return &Witness{
		ws:             ws,
		gcp:            gcp,
		logURL:         logURL,
		logSigVerifier: logSigVerifier,
		pollInterval:   pollInterval,
	}, nil
}

// getCheckpoint returns a checkpoint which is registered with witness
func (s *Witness) getCheckpoint(w http.ResponseWriter, r *http.Request) {
	s.witnessLock.Lock()
	w.Header().Set("Content-Type", "text/plain")
	w.Write(s.gcp.Envelope)
}

// RegisterHandlers registers HTTP handlers for firmware transparency endpoints.
func (s *Witness) RegisterHandlers(r *mux.Router) {
	r.HandleFunc(fmt.Sprintf("/%s", api.WitnessGetCheckpoint), s.getCheckpoint).Methods("GET")
}

// Poll periodically polls the FT log for updating the witness checkpoint.
// It only returns on error (when it doesn't start its own polling thread)
func (s *Witness) Poll(ctx context.Context) error {
	ftURL, err := url.Parse(s.logURL)
	if err != nil {
		return fmt.Errorf("failed to parse FT log URL: %w", err)
	}
	c := client.ReadonlyClient{
		LogURL:         ftURL,
		LogSigVerifier: s.logSigVerifier,
	}
	follow := client.NewLogFollower(c)

	glog.Infof("Polling FT log %q...", ftURL)
	cpc, cperrc := follow.Checkpoints(ctx, s.pollInterval, s.gcp)

	for {
		var cp api.LogCheckpoint
		select {
		case err = <-cperrc:
			return err
		case <-ctx.Done():
			return ctx.Err()
		case cp = <-cpc:
		}

		s.witnessLock.Lock()
		if err = s.ws.StoreCP(cp.Envelope); err != nil {
			glog.Warningf("Failed to save new logcheckpoint into store: %q", err)
			s.witnessLock.Unlock()
			continue
		}
		s.gcp = cp
		s.witnessLock.Unlock()
	}
}
