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

// Package impl is the implementation of the Firmware Transparency map server.
package impl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/ftmap"
	"github.com/google/trillian/experimental/batchmap"

	"github.com/gorilla/mux"

	_ "github.com/mattn/go-sqlite3" // Load drivers for sqlite3
)

type MapReader interface {
	// LatestRevision gets the metadata for the last completed write.
	LatestRevision() (rev int, logroot []byte, count int64, err error)

	// Tile gets the tile at the given path in the given revision of the map.
	Tile(revision int, path []byte) (*batchmap.Tile, error)
}

// MapServerOpts encapsulates options for running an FT map server.
type MapServerOpts struct {
	ListenAddr string
	MapDBAddr  string
}

func Main(ctx context.Context, opts MapServerOpts) error {
	if len(opts.MapDBAddr) == 0 {
		return errors.New("map DB is required")
	}
	mapDB, err := ftmap.NewMapDB(opts.MapDBAddr)
	if err != nil {
		return fmt.Errorf("failed to open map DB at %q: %v", opts.MapDBAddr, err)
	}

	glog.Infof("Starting FT map server...")
	srv := Server{db: mapDB}
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

// Server is the core state & handler implementation of the FT personality.
type Server struct {
	db MapReader
}

// getCheckpoint returns a recent MapCheckpoint.
func (s *Server) getCheckpoint(w http.ResponseWriter, r *http.Request) {
	rev, root, count, err := s.db.LatestRevision()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	glog.V(1).Infof("Latest revision: %d", rev)
	tile, err := s.db.Tile(rev, []byte{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	checkpoint := api.MapCheckpoint{
		LogCheckpoint: root,
		LogSize:       uint64(count),
		Revision:      uint64(rev),
		RootHash:      tile.RootHash,
	}
	js, err := json.Marshal(checkpoint)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

// RegisterHandlers registers HTTP handlers for the endpoints.
func (s *Server) RegisterHandlers(r *mux.Router) {
	r.HandleFunc(fmt.Sprintf("/%s", api.MapHTTPGetCheckpoint), s.getCheckpoint).Methods("GET")
}
