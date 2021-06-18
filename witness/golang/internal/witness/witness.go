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

// Package witness is designed to make sure the checkpoints of verifiable logs
// are consistent and store/serve/sign them if so.  It is expected that a separate
// feeder component would be responsible for the actual interaction with logs.
package witness

import (
	"context"
	"fmt"

	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/merkle/logverifier"
	"golang.org/x/mod/sumdb/note"
)

// Chkpt consists of the size of a checkpoint and its raw format.
type Chkpt struct {
	Size uint64
	Raw  []byte
}

// ChkptStorage is an interface for reading and writing checkpoints.
type ChkptStorage interface {
	// GetLatest returns the latest checkpoint for a given log.
	GetLatest(logID string) (Chkpt, error)

	// SetCheckpoint adds a checkpoint to the storage for a given log with
	// latestSize as the size of its largest stored checkpoint.
	SetCheckpoint(ctx context.Context, logID string, latestSize uint64, c Chkpt) error
}

// Opts is the options passed to a witness.
type Opts struct {
	Storage   ChkptStorage
	Signer    note.Signer
	KnownLogs map[string]LogInfo
}

// LogInfo consists of the information needed to verify the signatures and
// consistency proofs of a given log.
type LogInfo struct {
	// TODO(smeiklej) can probably remove this field since it is already
	// implicit in the log verifier.
	Hasher hashers.LogHasher
	SigVs  []note.Verifier
	LogV   logverifier.LogVerifier
}

// A witness consists of a checkpoint storage mechanism, a signer, and a list of
// logs for which it stores and verifies checkpoints.
type Witness struct {
	db     ChkptStorage
	Signer note.Signer
	// At some point we might want to store this information in a table in
	// the database too but as I imagine it being populated from a static
	// config file it doesn't seem very urgent to do that.
	Logs map[string]LogInfo
}

// NewWitness creates a new witness, which initially has no logs to follow.
func New(wo *Opts) *Witness {
	return &Witness{
		db:     wo.Storage,
		Signer: wo.Signer,
		Logs:   wo.KnownLogs,
	}
}

// parse verifies the checkpoint under the appropriate keys for logID and returns
// the parsed checkpoint.
func (w *Witness) parse(chkptRaw []byte, logID string) (*log.Checkpoint, error) {
	logInfo, ok := w.Logs[logID]
	if !ok {
		return nil, fmt.Errorf("no information for log %q", logID)
	}
	n, err := note.Open(chkptRaw, note.VerifierList(logInfo.SigVs...))
	if err != nil {
		return nil, fmt.Errorf("failed to verify checkpoint: %v", err)
	}
	chkpt := &log.Checkpoint{}
	if _, err := chkpt.Unmarshal([]byte(n.Text)); err != nil {
		return nil, fmt.Errorf("failed to unmarshal new checkpoint: %v", err)
	}
	return chkpt, nil
}

// GetCheckpoint gets a checkpoint for a given log, which will be
// consistent with all other checkpoints for the same log.  It also signs it
// under the witness' key.
func (w *Witness) GetCheckpoint(logID string) ([]byte, error) {
	chkpt, err := w.db.GetLatest(logID)
	if err != nil {
		return nil, err
	}
	// Add the witness' signature to the checkpoint.
	logInfo, ok := w.Logs[logID]
	if !ok {
		return nil, fmt.Errorf("no information for log %q", logID)
	}
	n, err := note.Open(chkpt.Raw, note.VerifierList(logInfo.SigVs...))
	if err != nil {
		return nil, fmt.Errorf("failed to verify checkpoint: %v", err)
	}
	cosigned, err := note.Sign(n, w.Signer)
	if err != nil {
		return nil, err
	}
	return cosigned, nil
}

// Update updates the latest checkpoint if chkptRaw is consistent with the current
// latest one for this log (according to latestSize).   It returns the latest
// checkpoint size before the update was applied (or just what was fetched if the
// update was unsuccessful).
func (w *Witness) Update(ctx context.Context, logID string, latestSize uint64, chkptRaw []byte, proof [][]byte) (uint64, error) {
	// Check the signatures on the raw checkpoint and parse it
	// into the log.Checkpoint format.
	chkpt, err := w.parse(chkptRaw, logID)
	if err != nil {
		return 0, err
	}
	// Get the latest one for the log because we don't want consistency proofs
	// with respect to older checkpoints.
	latest, err := w.db.GetLatest(logID)
	if err != nil {
		return 0, err
	}
	p, err := w.parse(latest.Raw, logID)
	if err != nil {
		return 0, err
	}
	// If they're out of date, let the caller know.
	if latestSize != p.Size {
		return p.Size, nil
	}
	if chkpt.Size > p.Size {
		logInfo, ok := w.Logs[logID]
		if !ok {
			return 0, fmt.Errorf("no information for log %q", logID)
		}
		if err := logInfo.LogV.VerifyConsistencyProof(int64(p.Size), int64(chkpt.Size), p.Hash, chkpt.Hash, proof); err != nil {
			// Complain if the checkpoints aren't consistent.
			return 0, fmt.Errorf("failed to verify consistency proof: %v", err)
		}
		// If the consistency proof is good we store chkptRaw.
		return p.Size, w.db.SetCheckpoint(ctx, logID, latest.Size, Chkpt{Size: chkpt.Size, Raw: chkptRaw})
	}
	// Complain if latest is bigger than chkpt.
	return 0, fmt.Errorf("Cannot prove consistency backwards")
}
