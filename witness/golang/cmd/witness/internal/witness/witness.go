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
	"database/sql"
	"fmt"

	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian/merkle/logverifier"
	"golang.org/x/mod/sumdb/note"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Chkpt contains the size of a checkpoint and its raw format.
type Chkpt struct {
	Size uint64
	Raw  []byte
}

// Opts is the options passed to a witness.
type Opts struct {
	DB        *sql.DB
	Signer    note.Signer
	KnownLogs map[string]LogInfo
}

// LogInfo contains the information needed to verify the signatures and
// consistency proofs of a given log.
type LogInfo struct {
	SigVs []note.Verifier
	LogV  logverifier.LogVerifier
}

// Witness consists of a database for storing checkpoints, a signer, and a list
// of logs for which it stores and verifies checkpoints.
type Witness struct {
	db     *sql.DB
	Signer note.Signer
	// At some point we might want to store this information in a table in
	// the database too but as I imagine it being populated from a static
	// config file it doesn't seem very urgent to do that.
	Logs map[string]LogInfo
}

// New creates a new witness, which initially has no logs to follow.
func New(wo Opts) (*Witness, error) {
	// Create the chkpts table if needed.
	_, err := wo.DB.Exec(`CREATE TABLE IF NOT EXISTS chkpts (
			      logID BLOB PRIMARY KEY,
			      size INT,
			      raw BLOB
			      )`)
	if err != nil {
		return nil, err
	}
	return &Witness{
		db:     wo.DB,
		Signer: wo.Signer,
		Logs:   wo.KnownLogs,
	}, nil
}

// parse verifies the checkpoint under the appropriate keys for logID and returns
// the parsed checkpoint.
func (w *Witness) parse(chkptRaw []byte, logID string) (*log.Checkpoint, error) {
	logInfo, ok := w.Logs[logID]
	if !ok {
		return nil, fmt.Errorf("log %q not found", logID)
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
	chkpt, err := w.getLatestChkpt(w.db.QueryRow, logID)
	if err != nil {
		return nil, err
	}
	// Add the witness' signature to the checkpoint.
	logInfo, ok := w.Logs[logID]
	if !ok {
		return nil, fmt.Errorf("log %q not found", logID)
	}
	n, err := note.Open(chkpt.Raw, note.VerifierList(logInfo.SigVs...))
	if err != nil {
		return nil, fmt.Errorf("failed to verify checkpoint: %v", err)
	}
	cosigned, err := note.Sign(n, w.Signer)
	if err != nil {
		return nil, fmt.Errorf("couldn't sign checkpoint: %v", err)
	}
	return cosigned, nil
}

// Update updates the latest checkpoint if chkptRaw is consistent with the current
// latest one for this log. It returns the latest checkpoint size before the
// update was applied (or just what was fetched if the update was unsuccessful).
func (w *Witness) Update(ctx context.Context, logID string, chkptRaw []byte, proof [][]byte) (uint64, error) {
	// If we don't witness this log then no point in going further.
	logInfo, ok := w.Logs[logID]
	if !ok {
		return 0, fmt.Errorf("log %q not found", logID)
	}
	// Check the signatures on the raw checkpoint and parse it
	// into the log.Checkpoint format.
	chkpt, err := w.parse(chkptRaw, logID)
	if err != nil {
		return 0, err
	}
	c := Chkpt{Size: chkpt.Size, Raw: chkptRaw}
	// Get the latest one for the log because we don't want consistency proofs
	// with respect to older checkpoints.  Bind this all in a transaction to
	// avoid race conditions when updating the database.
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("couldn't create tx: %v", err)
	}
	latest, err := w.getLatestChkpt(tx.QueryRow, logID)
	if err != nil {
		// If there was nothing stored already then treat this new
		// checkpoint as trust-on-first-use.
		if status.Code(err) == codes.NotFound {
			return 0, w.setCheckpoint(tx, logID, c)
		}
		return 0, fmt.Errorf("Update: %w", err)
	}
	p, err := w.parse(latest.Raw, logID)
	if err != nil {
		return 0, err
	}
	if chkpt.Size > p.Size {
		if err := logInfo.LogV.VerifyConsistencyProof(int64(p.Size), int64(chkpt.Size), p.Hash, chkpt.Hash, proof); err != nil {
			// Complain if the checkpoints aren't consistent.
			return p.Size, fmt.Errorf("failed to verify consistency proof: %v", err)
		}
		// If the consistency proof is good we store chkptRaw.
		return p.Size, w.setCheckpoint(tx, logID, c)
	}
	// Complain if latest is bigger than chkpt.
	return p.Size, fmt.Errorf("Cannot prove consistency backwards")
}

// getLatestChkpt returns the latest checkpoint for a given log.
func (w *Witness) getLatestChkpt(queryRow func(query string, args ...interface{}) *sql.Row, logID string) (Chkpt, error) {
	row := queryRow("SELECT raw, size FROM chkpts WHERE logID = ?", logID)
	if err := row.Err(); err != nil {
		return Chkpt{}, err
	}
	var maxChkpt Chkpt
	if err := row.Scan(&maxChkpt.Raw, &maxChkpt.Size); err != nil {
		if err == sql.ErrNoRows {
			return Chkpt{}, status.Errorf(codes.NotFound, "log %q not found", logID)
		}
		return Chkpt{}, err
	}
	return maxChkpt, nil
}

// setCheckpoint writes the checkpoint to the DB for a given log.
func (w *Witness) setCheckpoint(tx *sql.Tx, logID string, c Chkpt) error {
	tx.Exec(`INSERT INTO chkpts (logID, size, raw) VALUES (?, ?, ?)
		 ON CONFLICT(logID) DO
		 UPDATE SET size=excluded.size, raw=excluded.raw`,
		logID, c.Size, c.Raw)
	return tx.Commit()
}
