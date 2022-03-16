// Copyright 2022 Google LLC. All Rights Reserved.
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

// Package persistence defines interfaces and tests for storing log state.
package persistence

// LogStatePersistence is a handle on persistent storage for log state.
type LogStatePersistence interface {
	// Init sets up the persistence layer. This should be idempotent,
	// and will be called once per process startup.
	Init() error

	// Logs returns the IDs of all logs that have checkpoints that can
	// be read.
	Logs() ([]string, error)

	// ReadOps returns read-only operations for the given log ID. This
	// method only makes sense for IDs returned by Logs().
	ReadOps(logID string) (LogStateReadOps, error)

	// WriteOps shows intent to write data for the given logID. The
	// returned operations must have Close() called when the intent
	// is complete.
	// There is no requirement that the ID is present in Logs(); if
	// the ID is not there and this operation succeeds in committing
	// a checkpoint, then Logs() will return the new ID afterwards.
	WriteOps(logID string) (LogStateWriteOps, error)
}

// LogStateReadOps allows the data about a single log to be read.
type LogStateReadOps interface {
	// GetLatest returns the latest checkpoint and its compact range (if applicable).
	// If no checkpoint exists, it must return codes.NotFound.
	GetLatest() ([]byte, []byte, error)
}

// LogStateWriteOps allows data about a single log to be read and written
// in an ACID transaction. This naturally mirrors a DB transaction, but
// implementations don't need to use a database.
// Note that Close() must be called whenever these operations are no longer
// needed.
type LogStateWriteOps interface {
	LogStateReadOps

	// Set sets a new checkpoint and (optional) compact range
	// for the log. This commits the state to persistence.
	// After this call, only Close() should be called on this object.
	Set(checkpointRaw []byte, compactRange []byte) error

	// Terminates the write operation, freeing all resources.
	// This method MUST be called.
	Close()
}
