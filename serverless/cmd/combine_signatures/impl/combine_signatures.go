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

// impl provides the implementation of the combine_signatures tool.
package impl

import (
	"fmt"

	"golang.org/x/mod/sumdb/note"
)

// Combine returns checkpoint with the union of all signatures on the provided checkpoints from known witnesses.
// Signatures from unknown witnesses are discarded.
//
// All cps:
//  - MUST contain identical checkpoint bodies
//  - MUST be signed by the log whose verifier is provided.
//  - MAY be signed by one or more witnesses.
// if this isn't the case an error is returned.
func Combine(cps [][]byte, logSigV note.Verifier, witSigVs note.Verifiers) ([]byte, error) {
	var ret *note.Note

	for i, cp := range cps {
		// Ensure the Checkpoint is for the specific log
		candN, err := note.Open(cp, note.VerifierList(logSigV))
		if err != nil {
			return nil, fmt.Errorf("checkpoint %d is not signed by log: %v", i, err)
		}
		// if this is the first CP, then just take it, but remove any unknown/unwanted sigs.
		if ret == nil {
			ret = candN
			ret.UnverifiedSigs = nil
		}

		// Now gather witness sigs.
		// It's easier to just re-open with the verifiers we're interested in rather than trying to
		// dig through note.Sigs separating the log sig from the witnesses.
		candN, err = note.Open(cp, witSigVs)
		if err != nil {
			nErr, ok := err.(*note.UnverifiedNoteError)
			if !ok {
				return nil, fmt.Errorf("failed to open checkpoint %d: %v", i, err)
			}
			// Continue running
			candN = nErr.Note
		}

		if candN.Text != ret.Text {
			return nil, fmt.Errorf("checkpoint %d has differing content", i)
		}

		ret.Sigs = append(ret.Sigs, candN.Sigs...)
	}

	return note.Sign(ret)
}
