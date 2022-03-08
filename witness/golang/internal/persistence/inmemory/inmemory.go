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
	"fmt"
	"reflect"
	"sync"

	"github.com/google/trillian-examples/witness/golang/internal/persistence"
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
	mu          sync.RWMutex
	checkpoints map[string]checkpointState
}

func (p *inMemoryPersistence) Init() error {
	return nil
}

func (p *inMemoryPersistence) Logs() ([]string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	res := make([]string, 0, len(p.checkpoints))
	for k := range p.checkpoints {
		res = append(res, k)
	}
	return res, nil
}

func (p *inMemoryPersistence) ReadOps(logID string) (persistence.LogStateReadOps, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var cp *checkpointState
	if got, ok := p.checkpoints[logID]; ok {
		cp = &got
	}
	return &readWriter{
		read: cp,
	}, nil
}

func (p *inMemoryPersistence) WriteOps(logID string) (persistence.LogStateWriteOps, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var cp *checkpointState
	if got, ok := p.checkpoints[logID]; ok {
		cp = &got
	}
	return &readWriter{
		write: func(old *checkpointState, new checkpointState) error {
			return p.expectAndWrite(logID, old, new)
		},
		read: cp,
	}, nil
}

func (p *inMemoryPersistence) expectAndWrite(logID string, old *checkpointState, new checkpointState) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	got, found := p.checkpoints[logID]

	// Detect the possible conflicts
	if old != nil {
		if !found {
			return fmt.Errorf("expected old state %v but no state found when updating log %s", *old, logID)
		}
		if !reflect.DeepEqual(*old, got) {
			return fmt.Errorf("expected old state %v but got %s when updating log %s", *old, got, logID)
		}
	} else {
		if found {
			return fmt.Errorf("expected no state but found %v when updating log %s", got, logID)
		}
	}

	p.checkpoints[logID] = new
	return nil
}

type readWriter struct {
	write func(*checkpointState, checkpointState) error

	read, toStore *checkpointState
}

func (rw *readWriter) GetLatestCheckpoint() ([]byte, []byte, error) {
	if rw.read == nil {
		return nil, nil, status.Error(codes.NotFound, "no checkpoint found")
	}
	return rw.read.rawChkpt, rw.read.compactRange, nil
}

func (rw *readWriter) SetCheckpoint(c []byte, rng []byte) error {
	rw.toStore = &checkpointState{
		rawChkpt:     c,
		compactRange: rng,
	}
	return nil
}

func (rw *readWriter) Commit() error {
	return rw.write(rw.read, *rw.toStore)
}

func (rw *readWriter) Close() {
}
