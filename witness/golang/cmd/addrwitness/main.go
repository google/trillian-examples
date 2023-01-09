// Copyright 2021 Google LLC. All Rights Reserved.
// Copyright 2023 Filippo Valsorda. All Rights Reserved.
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

// Command addrwitness is an addressable witness, an HTTP service exposed to the
// Internet that accepts checkpoints for known logs and produces co-signatures.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"github.com/golang/glog"
	"github.com/gorilla/mux"
	_ "github.com/mattn/go-sqlite3"
	"github.com/transparency-dev/merkle/rfc6962"
	"golang.org/x/mod/sumdb/note"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v2"

	"github.com/google/trillian-examples/formats/log"
	wsql "github.com/google/trillian-examples/witness/golang/internal/persistence/sql"
	"github.com/google/trillian-examples/witness/golang/internal/witness"
)

var (
	listenAddr = flag.String("listen", "localhost:8000", "address:port to listen for requests on")
	dbFile     = flag.String("db", ":memory:", "path to a file to be used as sqlite3 storage for checkpoints")
	configFile = flag.String("config", "example_config.yaml", "path to a YAML config file that specifies the logs followed by this witness")
	skFile     = flag.String("key", "example_key.txt", "private signing key for the witness")
)

func main() {
	flag.Parse()

	if *skFile == "" {
		glog.Exit("--key must not be empty")
	}
	witnessSK, err := os.ReadFile(*skFile)
	if err != nil {
		glog.Exitf("Failed to read the private key: %v", err)
	}
	signer, err := note.NewSigner(strings.TrimSpace(string(witnessSK)))
	if err != nil {
		glog.Exitf("Error forming a signer: %v", err)
	}

	if len(*configFile) == 0 {
		glog.Exit("--config must not be empty")
	}
	fileData, err := os.ReadFile(*configFile)
	if err != nil {
		glog.Exitf("Failed to read from config file: %v", err)
	}
	var config LogConfig
	if err := yaml.Unmarshal(fileData, &config); err != nil {
		glog.Exitf("Failed to parse config file as proper YAML: %v", err)
	}
	if len(config.Logs) == 0 {
		glog.Exitf("No logs configured")
	}

	if len(*dbFile) == 0 {
		glog.Exit("--db must not be empty")
	}
	glog.Infof("Connecting to local DB at %q", *dbFile)
	db, err := sql.Open("sqlite3", *dbFile)
	if err != nil {
		glog.Exitf("Failed to connect to DB: %w", err)
	}
	// Avoid "database locked" errors with multiple concurrent updates.
	db.SetMaxOpenConns(1)

	logMap := make(map[string]witness.LogInfo)
	for _, log := range config.Logs {
		// We key the log map by Origin because there MUST be only one log per
		// Origin, since the Origin is the only log identifier that is signed as
		// part of the witness' cosignature. If two separate logs had the same
		// Origin, an attacker could move a co-signature from log A to log B,
		// and a log B client would accept it as valid, potentially hiding
		// entries from a log B monitor.
		//
		// To enable seamless key rotation, it might be desirable to support
		// multiple public keys for the same log during the transition period
		// (this is also the reason to support Origin lines separately from
		// public keys), but those public keys must refer to the same stored log
		// state.
		if _, ok := logMap[log.Origin]; ok {
			glog.Exitf("Duplicate log origin: %q", log.Origin)
		}

		v, err := note.NewVerifier(log.PublicKey)
		if err != nil {
			glog.Exitf("Failed to create signature verifier: %v", err)
		}
		logMap[log.Origin] = witness.LogInfo{
			SigV:       v,
			Origin:     log.Origin,
			Hasher:     rfc6962.DefaultHasher,
			UseCompact: false,
		}
	}

	w, err := witness.New(witness.Opts{
		Persistence: wsql.NewPersistence(db),
		Signer:      signer,
		KnownLogs:   logMap,
	})
	if err != nil {
		glog.Exitf("Error creating witness: %v", err)
	}

	glog.Infof("Starting witness server at http://%v...", *listenAddr)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	srv := NewServer(w)
	r := mux.NewRouter()
	srv.RegisterHandlers(r)
	hServer := &http.Server{
		Addr:        *listenAddr,
		Handler:     r,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	e := make(chan error, 1)
	go func() { e <- hServer.ListenAndServe() }()
	select {
	case <-ctx.Done():
		glog.Info("Server shutting down...")
		hServer.Shutdown(ctx)
	case err := <-e:
		glog.Errorf("Server error: %v", err)
	}
}

type LogConfig struct {
	Logs []struct {
		Origin    string `yaml:"Origin"`
		PublicKey string `yaml:"PublicKey"`
	} `yaml:"Logs"`
}

// Server is the core handler implementation of the witness.
type Server struct {
	w *witness.Witness
}

// NewServer creates a new Server.
func NewServer(witness *witness.Witness) *Server {
	return &Server{
		w: witness,
	}
}

// update handles requests to update checkpoints. It expects a POSTed body
// containing a JSON-formatted [UpdateRequest] statement and returns a the
// checkpoint with the additional signature.
func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("cannot read request body: %v", err), http.StatusBadRequest)
		return
	}
	var req UpdateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, fmt.Sprintf("cannot parse request body as proper JSON struct: %v", err), http.StatusBadRequest)
		return
	}
	unverifiedCheckpoint := new(log.Checkpoint)
	if _, err := unverifiedCheckpoint.Unmarshal(req.Checkpoint); err != nil {
		http.Error(w, fmt.Sprintf("failed to parse checkpoint: %v", err), http.StatusInternalServerError)
		return
	}
	// TODO: the logID should not be passed separately to prevent the signature
	// re-binding describe above. The database should also not store the whole
	// checkpoint to prevent denial of service attacks involving an excessive
	// number of large signatures.
	chkpt, err := s.w.Update(r.Context(), unverifiedCheckpoint.Origin, req.Checkpoint, req.Proof)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to update to new checkpoint: %v", err), httpForCode(status.Code(err)))
		return
	}
	// TODO: return only the additional signature, as otherwise logs will have
	// to check that the witness did not tamper with the checkpoint.
	w.Header().Set("Content-Type", "text/plain")
	w.Write(chkpt)
}

type UpdateRequest struct {
	Checkpoint []byte
	Proof      [][]byte
}

// getSize returns the tree size of the latest checkpoint known to the witness
// for a given tree, as an ASCII decimal. This is used to prepare the correct
// consistency proof for an update request. No other details of the tree head or
// co-signature are returned, to discourage the use of this endpoint by clients
// other than logs.
func (s *Server) getSize(w http.ResponseWriter, r *http.Request) {
	v := mux.Vars(r)
	logID := v["log"]
	// Get the signed checkpoint from the witness.
	chkpt, err := s.w.GetCheckpoint(logID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get checkpoint: %v", err), httpForCode(status.Code(err)))
		return
	}
	cp := new(log.Checkpoint)
	if _, err := cp.Unmarshal(chkpt); err != nil {
		http.Error(w, fmt.Sprintf("failed to parse checkpoint: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "%d\n", cp.Size)
}

// getLogs returns a list of all logs the witness is aware of.
func (s *Server) getLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := s.w.GetLogs()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get log list: %v", err), http.StatusInternalServerError)
		return
	}
	logList, err := json.Marshal(logs)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to convert log list to JSON: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(logList)
}

// RegisterHandlers registers HTTP handlers for witness endpoints.
func (s *Server) RegisterHandlers(r *mux.Router) {
	r.HandleFunc(`/witness/v0/logs/{log:[a-zA-Z0-9\-_=\. ]+}/size`, s.getSize).Methods("GET")
	r.HandleFunc("/witness/v0/update", s.update).Methods("PUT")
	r.HandleFunc("/witness/v0/logs", s.getLogs).Methods("GET")
}

func httpForCode(c codes.Code) int {
	switch c {
	case codes.AlreadyExists:
		return http.StatusConflict
	case codes.NotFound:
		return http.StatusNotFound
	case codes.FailedPrecondition, codes.InvalidArgument:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
