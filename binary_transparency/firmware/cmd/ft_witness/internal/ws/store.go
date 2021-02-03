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

// Package ws contains a Witness Store
package ws

import (
	"encoding/json"
	"log"
	"os"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
)

// Wstorage is a WS intended for storing witness checkpoints
// Currently a simple file is used as a storage mechanism
type Wstorage struct {
	fp string
}

// NewWstorage creates a new WS that uses the given file as DB backend
// The DB will be initialized if needed.
func NewWstorage(fp string) (*Wstorage, error) {
	ws := &Wstorage{
		fp: fp,
	}
	return ws, ws.init()
}

// init creates the file storage
func (ws *Wstorage) init() error {
	// Check if the file exists, if not create one
	f, err := os.OpenFile(ws.fp, os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
	return err
}

//StoreCP saves the given checkpoint into DB.
func (ws *Wstorage) StoreCP(wcp api.LogCheckpoint) error {
	// Check if file exists, open for write and store the checkpoint
	f, err := os.OpenFile(ws.fp, os.O_RDWR, 0755)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	data, err := json.MarshalIndent(wcp, "", " ")
	if err != nil {
		log.Fatalf("JSON marshaling failed: %s", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close() // ignore error; Write error takes precedence
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
	return err
}

// Retrieve gets the checkpoint previously stored.
func (ws *Wstorage) RetrieveCP() (api.LogCheckpoint, error) {
	// Check if the file exists, open for read
	f, err := os.OpenFile(ws.fp, os.O_RDONLY, 0755)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	var wcp api.LogCheckpoint
	if err := json.NewDecoder(f).Decode(&wcp); err != nil {
		glog.Exitf("Failed to parse witness log checkpoint file: %q", err)
	}
	return wcp, nil
}
