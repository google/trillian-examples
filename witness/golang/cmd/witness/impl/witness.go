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

// Package impl is the implementation of the witness server.
package impl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/golang/glog"
	ih "github.com/google/trillian-examples/witness/golang/cmd/witness/internal/http"
	"github.com/google/trillian-examples/witness/golang/cmd/witness/internal/witness"
	"github.com/gorilla/mux"
	"golang.org/x/mod/sumdb/note"

	_ "github.com/google/trillian/merkle/rfc6962/hasher" // Load hashers
	_ "github.com/mattn/go-sqlite3"                      // Load drivers for sqlite3
)

func Main(ctx context.Context, opts witness.Opts) error {
	w, err := witness.New(opts)
	if err != nil {
		return fmt.Errorf("error running witness: %q", err)
	}

	glog.Infof("Starting witness server...")
	srv := ih.NewServer(witness)
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
