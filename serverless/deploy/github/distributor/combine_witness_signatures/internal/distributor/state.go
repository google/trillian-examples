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

// Package distributor contains tooling for managing distributor state.
package distributor

import (
	"fmt"
	"sort"

	"github.com/google/trillian-examples/formats/checkpoints"
	"github.com/google/trillian-examples/formats/log"
	"golang.org/x/mod/sumdb/note"
)

// UpdateOpts contains the settings for the distributor update.
type UpdateOpts struct {
	LogSigV              note.Verifier
	Witnesses            []note.Verifier
	MaxWitnessSignatures uint
}

// UpdateState incorporates any incoming checkpoints into the distributor state.
func UpdateState(state, incoming [][]byte, opts UpdateOpts) ([][]byte, error) {
	// Handle changes in number of required CPs
	requiredSlots := int(opts.MaxWitnessSignatures + 1)
	if l := len(state); l > requiredSlots {
		state = state[0:requiredSlots]
	} else if l < requiredSlots {
		x := requiredSlots - l
		state = append(state, make([][]byte, x)...)
	}

	lSigV := note.VerifierList(opts.LogSigV)
	byBody := make(map[string][][]byte)
	for _, raw := range append(append([][]byte{}, state...), incoming...) {
		if len(raw) > 0 {
			n, err := note.Open(raw, lSigV)
			if err != nil {
				return nil, err
			}
			byBody[n.Text] = append(byBody[n.Text], raw)
		}
	}
	wSigV := note.VerifierList(opts.Witnesses...)
	allSigV := note.VerifierList(append([]note.Verifier{opts.LogSigV}, opts.Witnesses...)...)
	combined := make([]cpNoteRaw, 0, requiredSlots)
	for _, v := range byBody {
		raw, err := checkpoints.Combine(v, opts.LogSigV, wSigV)
		if err != nil {
			return nil, err
		}
		cp, n, err := open(raw, allSigV)
		if err != nil {
			return nil, err
		}
		combined = append(combined, cpNoteRaw{
			cp:   cp,
			note: n,
			raw:  raw,
		})
	}
	sort.Slice(combined, func(i, j int) bool {
		return combined[i].cp.Size > combined[j].cp.Size
	})

	c := 0
	ret := make([][]byte, len(state))
	// Recreate our set of witnessed checkpoints
nextCheckpoint:
	for i := range ret {
		for c < len(combined) {
			// Note - Sigs contains one extra sig, from the log.
			if len(combined[c].note.Sigs) > i {
				ret[i] = combined[c].raw
				continue nextCheckpoint
			}
			c++
		}
		// No checkpoints with at least i witness signatures
		ret[i] = nil
	}
	return ret, nil

}

func open(cpRaw []byte, sigV note.Verifiers) (*log.Checkpoint, *note.Note, error) {
	n, err := note.Open(cpRaw, sigV)
	if err != nil {
		return nil, nil, fmt.Errorf("checkpoint not signed by any known key: %v", err)
	}
	cp := &log.Checkpoint{}
	if _, err := cp.Unmarshal([]byte(n.Text)); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal: %v", err)
	}
	return cp, n, nil
}

type cpNoteRaw struct {
	cp   *log.Checkpoint
	note *note.Note
	raw  []byte
}
