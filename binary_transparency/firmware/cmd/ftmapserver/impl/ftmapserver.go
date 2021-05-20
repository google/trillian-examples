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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/ftmap"
	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian/experimental/batchmap"
	"github.com/google/trillian/types"

	"github.com/gorilla/mux"

	_ "github.com/mattn/go-sqlite3" // Load drivers for sqlite3
)

type MapReader interface {
	// LatestRevision gets the metadata for the last completed write.
	LatestRevision() (rev int, logroot types.LogRootV1, count int64, err error)

	// Tile gets the tile at the given path in the given revision of the map.
	Tile(revision int, path []byte) (*batchmap.Tile, error)

	// Aggregation gets the aggregation for the firmware at the given log index.
	Aggregation(revision int, fwLogIndex uint64) (api.AggregatedFirmware, error)
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
	rev, logRootV1, count, err := s.db.LatestRevision()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	glog.V(1).Infof("Latest revision: %d %+v", rev, logRootV1)
	tile, err := s.db.Tile(rev, []byte{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	lcp := api.LogCheckpoint{
		Checkpoint: log.Checkpoint{
			Ecosystem: api.FTLogCheckpointEcosystemv0,
			Size:      logRootV1.TreeSize,
			Hash:      logRootV1.RootHash,
		},
		TimestampNanos: logRootV1.TimestampNanos,
	}
	checkpoint := api.MapCheckpoint{
		LogCheckpoint: lcp.Marshal(),
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

// getTile returns the tile at the given revision & path.
func (s *Server) getTile(w http.ResponseWriter, r *http.Request) {
	rev, err := parseUintParam(r, "revision")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	path, err := parseBase64Param(r, "path")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	bmt, err := s.db.Tile(int(rev), path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	leaves := make([]api.MapTileLeaf, len(bmt.Leaves))
	for i, l := range bmt.Leaves {
		leaves[i] = api.MapTileLeaf{
			Path: l.Path,
			Hash: l.Hash,
		}
	}
	tile := api.MapTile{
		Path:   bmt.Path,
		Leaves: leaves,
	}
	js, err := json.Marshal(tile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

// getAggregation returns the aggregation for the firware at the given log index.
func (s *Server) getAggregation(w http.ResponseWriter, r *http.Request) {
	rev, err := parseUintParam(r, "revision")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	fwIndex, err := parseUintParam(r, "fwIndex")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	agg, err := s.db.Aggregation(int(rev), fwIndex)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	js, err := json.Marshal(agg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

// RegisterHandlers registers HTTP handlers for the endpoints.
func (s *Server) RegisterHandlers(r *mux.Router) {
	r.HandleFunc(fmt.Sprintf("/%s", api.MapHTTPGetCheckpoint), s.getCheckpoint).Methods("GET")
	// Empty tile path is normal for requesting the root tile
	r.HandleFunc(fmt.Sprintf("/%s/in-revision/{revision:[0-9]+}/at-path/", api.MapHTTPGetTile), s.getTile).Methods("GET")
	r.HandleFunc(fmt.Sprintf("/%s/in-revision/{revision:[0-9]+}/at-path/{path}", api.MapHTTPGetTile), s.getTile).Methods("GET")
	r.HandleFunc(fmt.Sprintf("/%s/in-revision/{revision:[0-9]+}/for-firmware-at-index/{fwIndex:[0-9]+}", api.MapHTTPGetAggregation), s.getAggregation).Methods("GET")
}

func parseBase64Param(r *http.Request, name string) ([]byte, error) {
	v := mux.Vars(r)
	b, err := base64.URLEncoding.DecodeString(v[name])
	if err != nil {
		return nil, fmt.Errorf("%s should be URL-safe base64 (%q)", name, err)
	}
	return b, nil
}

func parseUintParam(r *http.Request, name string) (uint64, error) {
	v := mux.Vars(r)
	i, err := strconv.ParseUint(v[name], 0, 64)
	if err != nil {
		return 0, fmt.Errorf("%s should be an integer (%q)", name, err)
	}
	return i, nil
}
