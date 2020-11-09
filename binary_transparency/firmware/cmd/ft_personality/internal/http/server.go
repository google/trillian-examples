// Package http contains private implementation details for the FirmwareTransparency personality server.
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/golang/glog"
	"github.com/google/trillian/types"
	"github.com/gorilla/mux"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
)

// Trillian is the interface to the Trillian Log required for the personality frontend.
type Trillian interface {
	// AddFirmwareManifest adds the firmware manifest to the log if it isn't already present.
	AddFirmwareManifest(ctx context.Context, data []byte) error

	// Root returns the most recent root seen by this client.
	Root() *types.LogRootV1

	// ConsistencyProof gets the consistency proof between two given tree sizes.
	ConsistencyProof(ctx context.Context, from, to uint64) ([][]byte, error)

	// FirmwareManifestAtIndex gets the value at the given index and an inclusion proof
	// to the given tree size.
	FirmwareManifestAtIndex(ctx context.Context, index, treeSize uint64) ([]byte, [][]byte, error)
}

// Server is the core state & handler implementation of the FT personality.
type Server struct {
	c Trillian
}

// NewServer creates a new server that interfaces with the given Trillian logger.
func NewServer(c Trillian) *Server {
	return &Server{
		c: c,
	}
}

// addFirmware handles requests to log new firmware images.
// It expects a mime/multipart POST consisting of FirmwareStatement.
// TODO(al): store the actual firmware image in a CAS too.
//
// Example usage:
// curl -i -X POST -H 'Content-Type: application/json' --data '@testdata/firmware_statement.json' localhost:8000/ft/v0/add-firmware
func (s *Server) addFirmware(w http.ResponseWriter, r *http.Request) {
	stmt := api.FirmwareStatement{}

	// Store the original bytes as statement to avoid a round-trip (de)serialization.
	statement, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := json.NewDecoder(bytes.NewReader(statement)).Decode(&stmt); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// "Verify" the signature:
	// TODO(al): do proper sigs
	if sigStr := string(stmt.Signature); sigStr != "LOL!" {
		http.Error(w, fmt.Sprintf("invalid LOL! sig %q", sigStr), http.StatusBadRequest)
		return
	}

	// Parse the firmware metadata:
	var meta api.FirmwareMetadata
	if err := json.Unmarshal(stmt.Metadata, &meta); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	glog.V(1).Infof("Got firmware %+v", meta)
	if err := s.c.AddFirmwareManifest(r.Context(), statement); err != nil {
		http.Error(w, fmt.Sprintf("failed to log firmware to Trillian %v", err), http.StatusInternalServerError)
	}

	w.Header().Set("Content-Type", "application/json")
}

func parseIntParam(r *http.Request, name string) (uint64, error) {
	v := mux.Vars(r)
	i, err := strconv.ParseUint(v[name], 0, 64)
	if err != nil {
		return 0, fmt.Errorf("%s should be an integer (%q)", name, err)
	}
	return i, nil
}

// getConsistency returns consistency proofs between published tree sizes.
func (s *Server) getConsistency(w http.ResponseWriter, r *http.Request) {
	from, err := parseIntParam(r, "from")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	to, err := parseIntParam(r, "to")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validation on the tree sizes being requested.
	if from == 0 {
		http.Error(w, fmt.Sprintf("from %d must be larger than 0", from), http.StatusBadRequest)
		return
	}
	if from > to {
		http.Error(w, fmt.Sprintf("from %d > to %d", from, to), http.StatusBadRequest)
		return
	}
	goldenSize := s.c.Root().TreeSize
	if to > goldenSize {
		http.Error(w, fmt.Sprintf("requested tree size %d > current tree size %d", to, goldenSize), http.StatusBadRequest)
		return
	}

	// Tree sizes requested seem reasonable, so fetch and return the proof.
	proof, err := s.c.ConsistencyProof(r.Context(), from, to)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get consistency proof: %v", err), http.StatusInternalServerError)
		return
	}
	cp := api.ConsistencyProof{
		Proof: proof,
	}

	js, err := json.Marshal(cp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

// getManifestEntryAndProof returns a tree leaf and corresponding inclusion proof.
func (s *Server) getManifestEntryAndProof(w http.ResponseWriter, r *http.Request) {
	index, err := parseIntParam(r, "index")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	treeSize, err := parseIntParam(r, "treesize")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Validation on the tree sizes being requested.
	if index >= treeSize {
		http.Error(w, fmt.Sprintf("index %d >= treesize %d", index, treeSize), http.StatusBadRequest)
		return
	}
	goldenSize := s.c.Root().TreeSize
	if treeSize > goldenSize {
		http.Error(w, fmt.Sprintf("requested tree size %d > current tree size %d", treeSize, goldenSize), http.StatusBadRequest)
		return
	}

	// Tree sizes requested seem reasonable, so fetch and return the proof.
	data, proof, err := s.c.FirmwareManifestAtIndex(r.Context(), index, treeSize)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get leaf & inclusion proof: %v", err), http.StatusInternalServerError)
		return
	}
	cp := api.InclusionProof{
		Value: data,
		Proof: proof,
	}

	js, err := json.Marshal(cp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

// getRoot returns a recent tree root.
func (s *Server) getRoot(w http.ResponseWriter, r *http.Request) {
	sth := s.c.Root()
	checkpoint := api.LogCheckpoint{
		TreeSize:       sth.TreeSize,
		RootHash:       sth.RootHash,
		TimestampNanos: sth.TimestampNanos,
	}
	js, err := json.Marshal(checkpoint)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

// RegisterHandlers registers HTTP handlers for firmware transparency endpoints.
func (s *Server) RegisterHandlers(r *mux.Router) {
	r.HandleFunc(fmt.Sprintf("/%s", api.HTTPAddFirmware), s.addFirmware).Methods("POST")
	r.HandleFunc(fmt.Sprintf("/%s/from/{from:[0-9]+}/to/{to:[0-9]+}", api.HTTPGetConsistency), s.getConsistency).Methods("GET")
	r.HandleFunc(fmt.Sprintf("/%s/at/{index:[0-9]+}/in-tree-of/{treesize:[0-9]+}", api.HTTPGetManifestEntryAndProof), s.getManifestEntryAndProof).Methods("GET")
	r.HandleFunc(fmt.Sprintf("/%s", api.HTTPGetRoot), s.getRoot).Methods("GET")
}
