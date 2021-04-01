// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// witness starts a HTTP server that can return its latest Golden Checkpoint, and
// perform consistency checks for client-provided Checkpoints.
package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"net/http"
	"strconv"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/sumdbaudit/audit"
	"github.com/gorilla/mux"
)

var (
	height = flag.Int("h", 8, "tile height")
	vkey   = flag.String("k", "sum.golang.org+033de0ae+Ac4zctda0e5eza+HJyk9SxEdh+s3Ux18htTTAD8OuAn8", "key")

	listenAddr = flag.String("listen", ":8000", "address:port to listen for requests on")
)

type server struct {
	a *audit.Service
}

// getGolden returns the latest validated Checkpoint.
func (s *server) getGolden(w http.ResponseWriter, r *http.Request) {
	golden := s.a.GoldenCheckpoint(r.Context())
	if golden == nil {
		http.Error(w, "failed to find golden checkpoint!", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(golden.Raw)))
	w.Write(golden.Raw)
}

// checkConsistency validates whether the provided checkpoint is consistent
// with the local state.
func (s *server) checkConsistency(w http.ResponseWriter, r *http.Request) {
	rawCheckpoint, err := parseBase64Param(r, "cp")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	glog.V(2).Infof("got checkpoint %q", rawCheckpoint)
	checkpoint, err := s.a.ParseCheckpointNote(rawCheckpoint)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.a.CheckConsistency(r.Context(), checkpoint); err != nil {
		switch err.(type) {
		case audit.CheckpointTooFresh:
			http.Error(w, err.Error(), http.StatusBadRequest)
		case audit.InconsistentCheckpoints:
			// TODO(mhutchinson): Return both checkpoints in a parseable response
			http.Error(w, err.Error(), http.StatusTeapot)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}

	// TODO(mhutchinson): Synthesize and return a proof of consistency with Golden CP.
	w.Header().Set("Content-Type", "application/json")
}

func main() {
	flag.Parse()

	db, err := audit.NewDatabaseFromFlags()
	if err != nil {
		glog.Exitf("Failed to open DB: %v", err)
	}

	sumDB := audit.NewSumDB(*height, *vkey)
	auditor := audit.NewService(db, sumDB, *height)
	server := &server{a: auditor}

	glog.Infof("Starting witness listening on %s...", *listenAddr)

	r := mux.NewRouter()
	r.HandleFunc("/golden", server.getGolden).Methods("GET")
	r.HandleFunc("/checkConsistency/{cp}", server.checkConsistency).Methods("GET")
	glog.Fatal(http.ListenAndServe(*listenAddr, r))
}

func parseBase64Param(r *http.Request, name string) ([]byte, error) {
	v := mux.Vars(r)
	b, err := base64.URLEncoding.DecodeString(v[name])
	if err != nil {
		return nil, fmt.Errorf("%s should be URL-safe base64 (%q)", name, err)
	}
	return b, nil
}
