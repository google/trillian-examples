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

// Package http contains private implementation details for the FirmwareTransparency personality server.
package http

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/crypto"
	"github.com/google/trillian/types"
	"github.com/gorilla/mux"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Trillian is the interface to the Trillian Log required for the personality frontend.
type Trillian interface {
	// AddSignedStatement adds the statement to the log if it isn't already present.
	AddSignedStatement(ctx context.Context, data []byte) error

	// Root returns the most recent root seen by this client.
	Root() *types.LogRootV1

	// ConsistencyProof gets the consistency proof between two given tree sizes.
	ConsistencyProof(ctx context.Context, from, to uint64) ([][]byte, error)

	// FirmwareManifestAtIndex gets the value at the given index and an inclusion proof
	// to the given tree size.
	FirmwareManifestAtIndex(ctx context.Context, index, treeSize uint64) ([]byte, [][]byte, error)

	// InclusionProofByHash fetches an inclusion proof and index for the first leaf found with the specified hash, if any.
	InclusionProofByHash(ctx context.Context, hash []byte, treeSize uint64) (uint64, [][]byte, error)
}

// CAS is the interface to the Content Addressable Store for firmware images.
type CAS interface {
	// Store puts the image under the key.
	Store([]byte, []byte) error

	// Retrieve gets a binary image that was previously stored.
	// Must return status code NotFound if no such image exists.
	Retrieve([]byte) ([]byte, error)
}

// Server is the core state & handler implementation of the FT personality.
type Server struct {
	c   Trillian
	cas CAS
}

// NewServer creates a new server that interfaces with the given Trillian logger.
func NewServer(c Trillian, cas CAS) *Server {
	return &Server{
		c:   c,
		cas: cas,
	}
}

// addFirmware handles requests to log new firmware images.
// It expects a mime/multipart POST consisting of SignedStatement and then firmware bytes.
func (s *Server) addFirmware(w http.ResponseWriter, r *http.Request) {
	statement, image, err := parseAddFirmwareRequest(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to parse request: %q", err.Error()), http.StatusBadRequest)
		return
	}

	stmt := api.SignedStatement{}
	if err := json.NewDecoder(bytes.NewReader(statement)).Decode(&stmt); err != nil {
		http.Error(w, fmt.Sprintf("failed to decode statement: %q", err.Error()), http.StatusBadRequest)
		return
	}

	// Verify the signature:
	if err := crypto.Publisher.VerifySignature(stmt.Type, stmt.Statement, stmt.Signature); err != nil {
		http.Error(w, fmt.Sprintf("signature verification failed! %v", err), http.StatusBadRequest)
		return
	}
	if stmt.Type != api.FirmwareMetadataType {
		http.Error(w, fmt.Sprintf("Expected statement type %q, but got %q", api.FirmwareMetadataType, stmt.Type), http.StatusBadRequest)
		return
	}

	// Parse the firmware metadata:
	var meta api.FirmwareMetadata
	if err := json.Unmarshal(stmt.Statement, &meta); err != nil {
		http.Error(w, fmt.Sprintf("failed to unmarshal metadata: %q", err.Error()), http.StatusBadRequest)
		return
	}

	glog.V(1).Infof("Got firmware %+v", meta)

	// Check the firmware bytes matches the manifest
	h := sha512.Sum512(image)
	if !bytes.Equal(h[:], meta.FirmwareImageSHA512) {
		http.Error(w, fmt.Sprintf("uploaded image does not match SHA512 in metadata (%x != %x)", h[:], meta.FirmwareImageSHA512), http.StatusBadRequest)
		return
	}

	if err := s.cas.Store((h[:]), image); err != nil {
		http.Error(w, fmt.Sprintf("failed to store image in CAS %v", err), http.StatusInternalServerError)
	}
	if err := s.c.AddSignedStatement(r.Context(), statement); err != nil {
		http.Error(w, fmt.Sprintf("failed to log firmware to Trillian %v", err), http.StatusInternalServerError)
	}

	w.Header().Set("Content-Type", "application/json")
}

// parseAddFirmwareRequest returns the bytes for the SignedStatement, and the firmware image respectively.
func parseAddFirmwareRequest(r *http.Request) ([]byte, []byte, error) {
	h := r.Header["Content-Type"]
	if len(h) == 0 {
		return nil, nil, fmt.Errorf("no content-type header")
	}

	mediaType, mediaParams, err := mime.ParseMediaType(h[0])
	if err != nil {
		return nil, nil, err
	}
	if !strings.HasPrefix(mediaType, "multipart/") {
		return nil, nil, fmt.Errorf("expecting mime multipart body")
	}
	boundary := mediaParams["boundary"]
	if len(boundary) == 0 {
		return nil, nil, fmt.Errorf("invalid mime multipart header - no boundary specified")
	}
	mr := multipart.NewReader(r.Body, boundary)

	// Get firmware statement (JSON)
	p, err := mr.NextPart()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find firmware statement in request body: %v", err)
	}
	rawJSON, err := ioutil.ReadAll(p)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read body of firmware statement: %v", err)
	}

	// Get firmware binary image
	p, err = mr.NextPart()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find firmware image in request body: %v", err)
	}
	image, err := ioutil.ReadAll(p)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read body of firmware image: %v", err)
	}
	return rawJSON, image, nil
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

// getInclusionByHash returns an inclusion proof for the entry with the specified hash (if it exists).
func (s *Server) getInclusionByHash(w http.ResponseWriter, r *http.Request) {
	hash, err := parseBase64Param(r, "hash")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	treeSize, err := parseIntParam(r, "treesize")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	goldenSize := s.c.Root().TreeSize
	if treeSize > goldenSize {
		http.Error(w, fmt.Sprintf("requested tree size %d > current tree size %d", treeSize, goldenSize), http.StatusBadRequest)
		return
	}

	// Tree sizes requested seem reasonable, so fetch and return the proof.
	index, proof, err := s.c.InclusionProofByHash(r.Context(), hash, treeSize)
	if status.Code(err) == codes.NotFound {
		http.Error(w, fmt.Sprintf("leaf hash not found: %v", err), http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get inclusion proof: %v", err), http.StatusInternalServerError)
		return
	}
	cp := api.InclusionProof{
		LeafIndex: index,
		Proof:     proof,
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
		Value:     data,
		LeafIndex: index,
		Proof:     proof,
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

// getFirmwareImage returns a firmware image stored in the CAS.
func (s *Server) getFirmwareImage(w http.ResponseWriter, r *http.Request) {
	hash, err := parseBase64Param(r, "hash")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	image, err := s.cas.Retrieve(hash)
	if err != nil {
		http.Error(w, err.Error(), httpStatusForErr(err))
		return
	}

	w.Header().Set("Content-Type", "application/binary")
	w.Header().Set("Content-Length", strconv.Itoa(len(image)))
	w.Write(image)
}

// addAnnotationMalware handles requests to annotate a logged firmware with a malware annotation.
func (s *Server) addAnnotationMalware(w http.ResponseWriter, r *http.Request) {
	ss := api.SignedStatement{}

	// Store the original bytes as statement to avoid a round-trip (de)serialization.
	rawStmt, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := json.NewDecoder(bytes.NewReader(rawStmt)).Decode(&ss); err != nil {
		http.Error(w, fmt.Sprintf("failed to decode statement: %q", err.Error()), http.StatusBadRequest)
		return
	}

	if err := crypto.AnnotatorMalware.VerifySignature(ss.Type, ss.Statement, ss.Signature); err != nil {
		http.Error(w, fmt.Sprintf("signature verification failed! %v", err), http.StatusBadRequest)
		return
	}
	if ss.Type != api.MalwareStatementType {
		http.Error(w, fmt.Sprintf("expected statement type %q, but got %q", api.MalwareStatementType, ss.Type), http.StatusBadRequest)
		return
	}

	var malwareStmt api.MalwareStatement
	if err := json.Unmarshal(ss.Statement, &malwareStmt); err != nil {
		http.Error(w, fmt.Sprintf("failed to unmarshal MalwareStatement: %q", err.Error()), http.StatusBadRequest)
		return
	}

	glog.V(1).Infof("Got MalwareStatement %+v", malwareStmt)

	if err := s.c.AddSignedStatement(r.Context(), rawStmt); err != nil {
		http.Error(w, fmt.Sprintf("failed to log firmware to Trillian %v", err), http.StatusInternalServerError)
	}

	w.Header().Set("Content-Type", "application/json")
}

// httpStatusForErr maps status codes to HTTP errors.
func httpStatusForErr(e error) int {
	switch status.Code(e) {
	case codes.OK:
		return http.StatusOK
	case codes.NotFound:
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
	// unreachable
}

// RegisterHandlers registers HTTP handlers for firmware transparency endpoints.
func (s *Server) RegisterHandlers(r *mux.Router) {
	r.HandleFunc(fmt.Sprintf("/%s", api.HTTPAddFirmware), s.addFirmware).Methods("POST")
	r.HandleFunc(fmt.Sprintf("/%s", api.HTTPAddAnnotationMalware), s.addAnnotationMalware).Methods("POST")
	r.HandleFunc(fmt.Sprintf("/%s/from/{from:[0-9]+}/to/{to:[0-9]+}", api.HTTPGetConsistency), s.getConsistency).Methods("GET")
	r.HandleFunc(fmt.Sprintf("/%s/for-leaf-hash/{hash}/in-tree-of/{treesize:[0-9]+}", api.HTTPGetInclusion), s.getInclusionByHash).Methods("GET")
	r.HandleFunc(fmt.Sprintf("/%s/at/{index:[0-9]+}/in-tree-of/{treesize:[0-9]+}", api.HTTPGetManifestEntryAndProof), s.getManifestEntryAndProof).Methods("GET")
	r.HandleFunc(fmt.Sprintf("/%s/with-hash/{hash}", api.HTTPGetFirmwareImage), s.getFirmwareImage).Methods("GET")
	r.HandleFunc(fmt.Sprintf("/%s", api.HTTPGetRoot), s.getRoot).Methods("GET")
}

func parseBase64Param(r *http.Request, name string) ([]byte, error) {
	v := mux.Vars(r)
	b, err := base64.URLEncoding.DecodeString(v[name])
	if err != nil {
		return nil, fmt.Errorf("%s should be URL-safe base64 (%q)", name, err)
	}
	return b, nil
}

func parseIntParam(r *http.Request, name string) (uint64, error) {
	v := mux.Vars(r)
	i, err := strconv.ParseUint(v[name], 0, 64)
	if err != nil {
		return 0, fmt.Errorf("%s should be an integer (%q)", name, err)
	}
	return i, nil
}
