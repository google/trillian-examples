// Package http contains private implementation details for the FirmwareTransparency personality server.
package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/golang/glog"
	"github.com/google/trillian/client"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
)

// Server is the core state & handler implementation of the FT personality.
type Server struct {
	c *client.LogClient
}

// NewServer creates a new server that interfaces with the given Trillian logger.
func NewServer(c *client.LogClient) *Server {
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

	// TODO(mhutchinson): This blocks until the statement is integrated into the log.
	// This _may_ be OK for a demo, but it really should be split.
	s.c.AddLeaf(r.Context(), statement)
}

// getConsistency returns consistency proofs between published tree sizes.
func (s *Server) getConsistency(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// getManifestEntries returns the leaves in the tree.
func (s *Server) getManifestEntries(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// getRoot returns a recent tree root.
func (s *Server) getRoot(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// RegisterHandlers registers HTTP handlers for firmware transparency endpoints.
func (s *Server) RegisterHandlers() {
	http.HandleFunc(api.HTTPAddFirmware, s.addFirmware)
	http.HandleFunc(api.HTTPGetConsistency, s.getConsistency)
	http.HandleFunc(api.HTTPGetManifestEntries, s.getManifestEntries)
	http.HandleFunc(api.HTTPGetRoot, s.getRoot)
}
