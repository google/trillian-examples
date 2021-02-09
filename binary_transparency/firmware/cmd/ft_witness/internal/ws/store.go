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

// Package ws contains a Witness Store
package ws

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
)

const (
	fileMask = 0755
)

// Storage is a Witness Storage intended for storing witness checkpoints
// Currently a simple file is used as a storage mechanism
type Storage struct {
	fp        string
	storeLock sync.Mutex
}

// NewStorage creates a new WS that uses the given file as DB backend
// The DB will be initialized if needed.
func NewStorage(fp string) (*Storage, error) {
	ws := &Storage{
		fp: fp,
	}
	return ws, ws.init()
}

// init creates the file storage
func (ws *Storage) init() error {
	// Check if the file exists, if not create one
	f, err := os.OpenFile(ws.fp, os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}
	return nil
}

// StoreCP saves the given checkpoint into DB.
func (ws *Storage) StoreCP(wcp api.LogCheckpoint) error {

	ws.storeLock.Lock()
	defer ws.storeLock.Unlock()

	// Check if file exists, open for write and store the checkpoint
	f, err := os.OpenFile(ws.fp, os.O_RDWR, fileMask)
	if err != nil {
		f.Close()
		return fmt.Errorf("failed to open file: %w", err)
	}

	data, err := json.MarshalIndent(wcp, "", " ")
	if err != nil {
		return fmt.Errorf("JSON marshaling failed: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("failed to write data to witness db file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}
	return nil
}

// RetrieveCP gets the checkpoint previously stored.
func (ws *Storage) RetrieveCP() (api.LogCheckpoint, error) {
	var wcp api.LogCheckpoint

	ws.storeLock.Lock()
	defer ws.storeLock.Unlock()
	// Check if the file exists, open for read
	f, err := os.OpenFile(ws.fp, os.O_RDONLY, fileMask)
	if err != nil {
		f.Close()
		return wcp, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(&wcp); (err != nil) && (err != io.EOF) {
		return wcp, fmt.Errorf("Failed to parse witness log checkpoint file: %w", err)
	}
	return wcp, nil
}
