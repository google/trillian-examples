// Package http contains private implementation details for the FirmwareTransparency personality server.
package http

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
)

// Server is the core state & handler implementation of the FT personality.
type Server struct {
}

// addFirmware handles requests to log new firmware images.
// It expects a mime/multipart POST consisting of FirmwareStatement.
// TODO(al): store the actual firmware image in a CAS too.
//
// Example usage:
// curl -i -X POST -H 'Content-Type: application/json' --data '@testdata/firmware_statement.json' localhost:8000/ft/v0/add-firmware
func (s *Server) addFirmware(w http.ResponseWriter, r *http.Request) {
	stmt := api.FirmwareStatement{}
	if err := json.NewDecoder(r.Body).Decode(&stmt); err != nil {
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
}

// getConsistency returns consistency proofs between published tree sizes.
func (s *Server) getConsistency(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// getFirmwareEntries returns the leaves in the tree.
func (s *Server) getFirmwareEntries(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// getRoot returns a recent tree root.
func (s *Server) getRoot(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// RegisterHandlers registers HTTP handlers for firmware transparency endpoints.
func (s *Server) RegisterHandlers() {
	http.HandleFunc("/ft/v0/add-firmware", s.addFirmware)
	http.HandleFunc("/ft/v0/get-consistency", s.getConsistency)
	http.HandleFunc("/ft/v0/get-firmware-entries", s.getFirmwareEntries)
	http.HandleFunc("/ft/v0/get-root", s.getRoot)
}
