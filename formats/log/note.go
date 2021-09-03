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

package log

import (
	"errors"
	"fmt"

	"golang.org/x/mod/sumdb/note"
)

// ParseCheckpoint takes a raw checkpoint as bytes and returns a parsed checkpoint
// providing that a valid log signature is found, the checkpoint unmarshals
// correctly, and the log origin is that expected. In all other cases, an empty
// checkpoint is returned. The underlying note is always returned where possible.
// The signatures on the note will include the log signature if no error is returned,
// plus any signatures from otherVerifiers that were found.
func ParseCheckpoint(origin string, logVerifier note.Verifier, otherVerifiers []note.Verifier, chkpt []byte) (Checkpoint, *note.Note, error) {
	vs := make([]note.Verifier, len(otherVerifiers)+1)
	vs[0] = logVerifier
	if n := copy(vs[1:], otherVerifiers); n < len(otherVerifiers) {
		return Checkpoint{}, nil, errors.New("failed to create verifier list")
	}
	verifiers := note.VerifierList(vs...)

	n, err := note.Open(chkpt, verifiers)
	if err != nil {
		return Checkpoint{}, n, fmt.Errorf("failed to verify signatures on checkpoint: %v", err)
	}

	for _, s := range n.Sigs {
		if s.Hash == logVerifier.KeyHash() && s.Name == logVerifier.Name() {
			// The log has signed this checkpoint. It is now safe to parse.
			cp := &Checkpoint{}
			if _, err := cp.Unmarshal([]byte(n.Text)); err != nil {
				return Checkpoint{}, n, fmt.Errorf("failed to unmarshal checkpoint: %v", err)
			}
			if cp.Origin != origin {
				return Checkpoint{}, n, fmt.Errorf("got Origin %q but expected %q", cp.Origin, origin)
			}
			return *cp, n, nil
		}
	}
	return Checkpoint{}, n, fmt.Errorf("no log signature found on note")
}
