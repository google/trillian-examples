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

// Package impl is the implementation of the Firmware Transparency personality server.
// This requires a Trillian instance to be reachable via gRPC and a tree to have
// been provisioned.
package impl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/cmd/ft_personality/internal/cas"
	ih "github.com/google/trillian-examples/binary_transparency/firmware/cmd/ft_personality/internal/http"
	"github.com/google/trillian-examples/binary_transparency/firmware/cmd/ft_personality/internal/trees"
	"github.com/google/trillian-examples/binary_transparency/firmware/cmd/ft_personality/internal/trillian"
	"github.com/gorilla/mux"
	"golang.org/x/mod/sumdb/note"

	_ "github.com/mattn/go-sqlite3" // Load drivers for sqlite3
)

// PersonalityOpts encapsulates options for running an FT personality.
type PersonalityOpts struct {
	ListenAddr     string
	CASFile        string
	TrillianAddr   string
	ConnectTimeout time.Duration
	STHRefresh     time.Duration
	Signer         note.Signer
}

func Main(ctx context.Context, opts PersonalityOpts) error {
	if len(opts.CASFile) == 0 {
		return errors.New("CAS file is required")
	}

	glog.Infof("Connecting to local DB at %q", opts.CASFile)
	db, err := sql.Open("sqlite3", opts.CASFile)
	if err != nil {
		return fmt.Errorf("failed to connect to DB: %w", err)
	}
	cas, err := cas.NewBinaryStorage(db)
	if err != nil {
		return fmt.Errorf("failed to connect CAS to DB: %w", err)
	}

	// TODO(mhutchinson): This is putting the tree config in the CAS DB.
	// This isn't unreasonable, but it does make the naming misleading now.
	treeStorage := trees.NewTreeStorage(db)

	glog.Infof("Connecting to Trillian Log...")
	tclient, err := trillian.NewClient(ctx, opts.ConnectTimeout, opts.TrillianAddr, treeStorage)
	if err != nil {
		return fmt.Errorf("failed to connect to Trillian: %w", err)
	}
	defer tclient.Close()

	// Periodically sync the golden STH in the background.
	go func() {
		for ctx.Err() == nil {
			if err := tclient.UpdateRoot(ctx); err != nil {
				glog.Warningf("error updating STH: %v", err)
			}

			select {
			case <-ctx.Done():
			case <-time.After(opts.STHRefresh):
			}
		}
	}()

	glog.Infof("Starting FT personality server...")
	srv := ih.NewServer(tclient, cas, opts.Signer)
	r := mux.NewRouter()
	srv.RegisterHandlers(r)
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
	hServer.Shutdown(ctx)
	return <-e
}
