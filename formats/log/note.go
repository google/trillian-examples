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
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"golang.org/x/mod/sumdb/note"
)

// CheckpointParser is responsible for parsing checkpoints for a specific log.
// Any valid note must have been signed by all verifiers known by this parser.
type CheckpointParser struct {
	logVerifier       note.Verifier
	logID             string
	otherVerifiers    note.Verifiers
	requiredOtherSigs int
}

// NewCheckpointParser creates a parser for checkpoints that must be signed by the log
// which is verified by logVKey, and *all* of the other verifiers under otherVerifiers.
func NewCheckpointParser(logVKey string, otherVerifiers ...string) (CheckpointParser, error) {
	logVerifier, err := note.NewVerifier(string(logVKey))
	if err != nil {
		return CheckpointParser{}, fmt.Errorf("NewVerifier(%s): %v", logVKey, err)
	}
	logVKHash := sha256.Sum256([]byte(logVKey))

	ovs := make([]note.Verifier, len(otherVerifiers))
	for i, k := range otherVerifiers {
		ovs[i], err = note.NewVerifier(k)
		if err != nil {
			return CheckpointParser{}, fmt.Errorf("NewVerifier(%s): %v", k, err)
		}
	}
	return CheckpointParser{
		logVerifier:       logVerifier,
		logID:             base64.StdEncoding.EncodeToString(logVKHash[:]),
		otherVerifiers:    note.VerifierList(ovs...),
		requiredOtherSigs: len(otherVerifiers),
	}, nil
}

// Parse returns a checkpoint parsed from the raw note, and any error if the note could
// not be parsed, or any of the required signatures were missing.
func (p CheckpointParser) Parse(chkptRaw []byte) (Checkpoint, error) {
	r := &Checkpoint{}
	n, err := note.Open(chkptRaw, note.VerifierList(p.logVerifier))
	if err != nil {
		return *r, fmt.Errorf("failed to verify log signature on checkpoint: %v", err)
	}
	if _, err := r.Unmarshal([]byte(n.Text)); err != nil {
		return *r, fmt.Errorf("failed to unmarshal new checkpoint: %v", err)
	}
	// TODO(mhutchinson): This is where we'd check the message contains p.logID

	if p.requiredOtherSigs == 0 {
		return *r, nil
	}

	if n, err = note.Open(chkptRaw, p.otherVerifiers); err != nil {
		return *r, fmt.Errorf("failed to verify required signatures on checkpoint: %v", err)
	}
	if len(n.Sigs) != p.requiredOtherSigs {
		return *r, fmt.Errorf("required %d non-log signatures but got %d", p.requiredOtherSigs, len(n.Sigs))
	}
	return *r, nil
}
