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
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/golang/glog"
	ih "github.com/google/trillian-examples/witness/golang/cmd/witness/internal/http"
	"github.com/google/trillian-examples/witness/golang/cmd/witness/internal/witness"
	"github.com/gorilla/mux"
	"github.com/google/trillian/merkle/logverifier"
	"github.com/google/trillian/merkle/rfc6962/hasher"
	_ "github.com/mattn/go-sqlite3" // Load drivers for sqlite3
	"golang.org/x/mod/sumdb/note"
)

type LogJSON struct {
	logs []LogInfoJSON
}

type LogInfoJSON struct {
	logID        string
	hashstrategy string
	pubkey       string
}

type ServerOpts struct {
	ListenAddr string
	DBFile     string
	Signer     note.Signer
	ConfigFile    string
}

func Main(ctx context.Context, opts ServerOpts) error {
	if len(opts.DBFile) == 0 {
		return errors.New("DB file is required")
	}
	if len(opts.ConfigFile) == 0 {
		return errors.New("Config file is required")
	}

	glog.Infof("Connecting to local DB at %q", opts.DBFile)
	db, err := sql.Open("sqlite3", opts.DBFile)
	if err != nil {
		return fmt.Errorf("failed to connect to DB: %w", err)
	}
	// Create log information from the config file.
	fileData, err := ioutil.ReadFile(opts.ConfigFile)
	if err != nil {
		return fmt.Errorf("failed to read from config file: %v", err)
	}
	var js LogJSON
	json.Unmarshal(fileData, &js)
	var logMap map[string]witness.LogInfo
	for _, log := range js.logs {
		h := hasher.DefaultHasher
		// TODO(smeiklej): Extend witness to handle other hashing strategies.
		if log.hashstrategy != "default" {
			return fmt.Errorf("can't handle non-default hashing strategies")
		}
		logV, err := note.NewVerifier(log.pubkey)
		if err != nil {
			return fmt.Errorf("failed to create signature verifier: %v", err)
		}
		sigVs := []note.Verifier{logV}
		logInfo := witness.LogInfo{
			SigVs: sigVs,
			LogV:  logverifier.New(h),
		}
		logMap[log.logID] = logInfo
	}

	w, err := witness.New(witness.Opts{
		Database:  db,
		Signer:    opts.Signer,
		KnownLogs: logMap,
	})
	if err != nil {
		return fmt.Errorf("error creating witness: %q", err)
	}

	glog.Infof("Starting witness server...")
	srv := ih.NewServer(ctx, w)
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
