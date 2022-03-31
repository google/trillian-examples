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

package storage

import (
	"errors"
	"fmt"
	"sync"

	"github.com/google/trillian-examples/witness/golang/internal/persistence"
	"github.com/google/trillian-examples/witness/golang/omniwitness/usbarmory/internal/storage/slots"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v3"
)

// SlotPersistence is an implementation of the witness Persistence
// interface based on Slots.
type SlotPersistence struct {
	mu   sync.Mutex
	part *slots.Partition

	// mapSlot is a reference to the zeroth slot in a partition.
	// This slot is used to maintain a mapping of log ID to slot index
	// where state for that log is stored.
	mapSlot       *slots.Slot
	mapWriteToken uint32

	idToSlot  slotMap
	freeSlots []uint
}

type slotMap map[string]uint

// NewSlotPeristence creates a new SlotPersistence instance.
func NewSlotPersistence(part *slots.Partition) *SlotPersistence {
	return &SlotPersistence{
		part:     part,
		idToSlot: make(map[string]uint),
	}
}

// populateMap reads the logID -> slot mapping from storage.
// Must be called with p.mu locked.
func (p *SlotPersistence) populateMap() error {
	b, t, err := p.mapSlot.Read()
	if err != nil {
		return fmt.Errorf("failed to read persistence mapping: %v", err)
	}
	if err := yaml.Unmarshal(b, &p.idToSlot); err != nil {
		return fmt.Errorf("failed to unmarshal persistence mapping: %v", err)
	}
	p.mapWriteToken = t
	slotState := make([]bool, p.part.NumSlots())
	for _, idx := range p.idToSlot {
		if idx == 0 {
			return errors.New("internal-error, reserved slot 0 has been used")
		}
		slotState[idx] = true
	}
	// Slot 0 is reserved.
	slotState[0] = true
	p.freeSlots = make([]uint, 0, p.part.NumSlots())
	for idx, used := range slotState {
		if !used {
			p.freeSlots = append(p.freeSlots, uint(idx))
		}
	}
	return nil
}

// storeMap writes the current logID -> slot map to storage.
// Must be called with p.mu locked.
func (p *SlotPersistence) storeMap() error {
	smRaw, err := yaml.Marshal(p.idToSlot)
	if err != nil {
		return fmt.Errorf("failed to marshal mapping: %v", err)
	}
	if err := p.mapSlot.CheckAndWrite(p.mapWriteToken, smRaw); err != nil {
		return fmt.Errorf("failed to store mapping: %v", err)
	}
	p.mapWriteToken++
	return nil
}

// addLog assigns a slot to a new log ID.
// Must be called with p.mu locked.
func (p *SlotPersistence) addLog(id string) (uint, error) {
	if idx, ok := p.idToSlot[id]; ok {
		return idx, nil
	}
	if len(p.freeSlots) == 0 {
		return 0, errors.New("no free slot available")
	}
	f := p.freeSlots[0]
	p.freeSlots = p.freeSlots[1:]
	p.idToSlot[id] = f
	return f, p.storeMap()
}

// Init sets up the persistence layer. This should be idempotent,
// and will be called once per process startup.
func (p *SlotPersistence) Init() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	s, err := p.part.Open(0)
	if err != nil {
		return fmt.Errorf("failed to open mapping slot 0: %v", err)
	}
	p.mapSlot = s
	if err := p.populateMap(); err != nil {
		return fmt.Errorf("failed to populate logID → slot map: %v", err)
	}
	return nil
}

// Logs returns the IDs of all logs that have checkpoints that can
// be read.
func (p *SlotPersistence) Logs() ([]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	r := make([]string, 0, len(p.idToSlot))
	for k := range p.idToSlot {
		r = append(r, k)
	}
	return r, nil
}

// ReadOps returns read-only operations for the given log ID. This
// method only makes sense for IDs returned by Logs().
func (p *SlotPersistence) ReadOps(logID string) (persistence.LogStateReadOps, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	i, ok := p.idToSlot[logID]
	if !ok {
		// TODO(al): work around undocumented assumptions in the storage interface;
		// we have to return a ReadOp even if we don't know about the specified logID.
		// the ReadOps we return must then return an error of codes.NotFound when GetLatest
		// is called so that the witness will eventually call WriteOps/Update with the
		// same logID and create the record.
		return &slotOps{}, nil
	}
	s, err := p.part.Open(i)
	if err != nil {
		return nil, fmt.Errorf("internal error opening slot %d associated with log ID %q: %v", i, logID, err)
	}
	return &slotOps{slot: s}, nil
}

// WriteOps shows intent to write data for the given logID. The
// returned operations must have Close() called when the intent
// is complete.
// There is no requirement that the ID is present in Logs(); if
// the ID is not there and this operation succeeds in committing
// a checkpoint, then Logs() will return the new ID afterwards.
func (p *SlotPersistence) WriteOps(logID string) (persistence.LogStateWriteOps, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	i, ok := p.idToSlot[logID]
	if !ok {
		var err error
		i, err = p.addLog(logID)
		if err != nil {
			return nil, fmt.Errorf("unable to assign slot for log ID %q: %v", logID, err)
		}
	}
	s, err := p.part.Open(i)
	if err != nil {
		return nil, fmt.Errorf("internal error opening slot %d associated with log ID %q: %v", i, logID, err)
	}
	return &slotOps{slot: s}, nil
}

type slotOps struct {
	mu         sync.Mutex
	slot       *slots.Slot
	writeToken uint32
}

type logRecord struct {
	Checkpoint []byte
	Proof      []byte
}

// GetLatest returns the latest checkpoint and its compact range (if applicable).
// If no checkpoint exists, it must return codes.NotFound.
func (s *slotOps) GetLatest() ([]byte, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// TODO(al): workaround for storage assumption - see comment in ReadOps above.
	if s.slot == nil {
		return nil, nil, status.Error(codes.NotFound, "no checkpoint for log")
	}

	b, t, err := s.slot.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read data: %v", err)
	}
	s.writeToken = t
	if len(b) == 0 {
		glog.V(2).Infof("No checkpoint")
		return nil, nil, status.Error(codes.NotFound, "no checkpoint for log")
	}
	lr := logRecord{}
	if err := yaml.Unmarshal(b, &lr); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal data: %v", err)
	}
	return lr.Checkpoint, lr.Proof, nil
}

// Set sets a new checkpoint and (optional) compact range
// for the log. This commits the state to persistence.
// After this call, only Close() should be called on this object.
func (s *slotOps) Set(checkpointRaw []byte, compactRange []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	lr := logRecord{
		Checkpoint: checkpointRaw,
		Proof:      compactRange,
	}

	lrRaw, err := yaml.Marshal(&lr)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %v", err)
	}

	if err := s.slot.CheckAndWrite(s.writeToken, lrRaw); err != nil {
		return fmt.Errorf("failed to write data: %v", err)
	}
	return nil
}

// Terminates the write operation, freeing all resources.
// This method MUST be called.
func (s *slotOps) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.slot = nil
}
