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

// Package impl is the implementation of the Firmware Transparency witness server.
// This requires a FT Log to be running at a known address.
package impl

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/golang/glog"
	ih "github.com/google/trillian-examples/binary_transparency/firmware/cmd/ft_witness/internal/http"
	"github.com/google/trillian-examples/binary_transparency/firmware/cmd/ft_witness/internal/ws"
	"github.com/gorilla/mux"
	"golang.org/x/mod/sumdb/note"
)

// WitnessOpts encapsulates options for running an FT witness.
type WitnessOpts struct {
	ListenAddr       string
	WSFile           string
	FtLogURL         string
	FtLogSigVerifier note.Verifier
	PollInterval     time.Duration
}

// Main kickstarts the witness
func Main(ctx context.Context, opts WitnessOpts) error {
	if len(opts.WSFile) == 0 {
		return errors.New("witness store file is required")
	}

	ws, err := ws.NewStorage(opts.WSFile)
	if err != nil {
		return fmt.Errorf("failed to connect witness store: %w", err)
	}

	glog.Infof("Starting FT witness server...")
	witness, err := ih.NewWitness(ws, opts.FtLogURL, opts.FtLogSigVerifier, opts.PollInterval)
	if err != nil {
		return fmt.Errorf("failed to create new witness: %w", err)
	}
	r := mux.NewRouter()
	witness.RegisterHandlers(r)

	go func() {
		if err := witness.Poll(ctx); err != nil {
			glog.Errorf("witness.Poll(): %v", err)
		}
	}()

	hServer := &http.Server{
		Addr:    opts.ListenAddr,
		Handler: r,
	}
	e := make(chan error, 1)
	go func() {
		e <- hServer.ListenAndServe()
		close(e)
	}()
	<-ctx.Done()
	glog.Info("Server shutting down")
	if err := hServer.Shutdown(ctx); err != nil {
		glog.Errorf("server.Shutdown(): %v", err)
	}
	return <-e
}
