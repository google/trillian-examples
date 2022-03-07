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

// Package inmemory provides a persistence implementation that lives only in memory.
package inmemory

import (
	"github.com/google/trillian-examples/witness/golang/cmd/witness/internal/persistence"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NewInMemoryPersistence returns a persistence object that lives only in memory.
func NewInMemoryPersistence() persistence.LogStatePersistence {
	return &inMemoryPersistence{
		checkpoints: make(map[string]checkpointState),
	}
}

type checkpointState struct {
	rawChkpt     []byte
	compactRange []byte
}

type inMemoryPersistence struct {
	checkpoints map[string]checkpointState
}

func (p *inMemoryPersistence) Init() error {
	return nil
}

func (p *inMemoryPersistence) Logs() ([]string, error) {
	res := make([]string, 0, len(p.checkpoints))
	for k := range p.checkpoints {
		res = append(res, k)
	}
	return res, nil
}

func (p *inMemoryPersistence) ReadOps(logID string) (persistence.LogStateReadOps, error) {
	return &ReadWriter{
		logID: logID,
		imp:   p,
	}, nil
}

func (p *inMemoryPersistence) WriteOps(logID string) (persistence.LogStateWriteOps, error) {
	return &ReadWriter{
		logID: logID,
		imp:   p,
	}, nil
}

type ReadWriter struct {
	logID string
	imp   *inMemoryPersistence

	temp checkpointState
}

func (rw *ReadWriter) GetLatestCheckpoint() ([]byte, []byte, error) {
	if cp, ok := rw.imp.checkpoints[rw.logID]; ok {
		return cp.rawChkpt, cp.compactRange, nil
	}
	return nil, nil, status.Errorf(codes.NotFound, "no checkpoint for log %q", rw.logID)
}

func (rw *ReadWriter) SetCheckpoint(c []byte, rng []byte) error {
	rw.temp = checkpointState{
		rawChkpt:     c,
		compactRange: rng,
	}
	return nil
}

func (rw *ReadWriter) Commit() error {
	rw.imp.checkpoints[rw.logID] = rw.temp
	return nil
}

func (rw *ReadWriter) Close() {
}
