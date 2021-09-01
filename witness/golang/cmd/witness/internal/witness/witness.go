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
	"bytes"
	"context"
	"database/sql"
	"fmt"

	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/merkle/logverifier"
	"golang.org/x/mod/sumdb/note"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Opts is the options passed to a witness.
type Opts struct {
	DB        *sql.DB
	Signer    note.Signer
	KnownLogs map[string]LogInfo
}

// LogInfo contains the information needed to verify log checkpoints.
type LogInfo struct {
	// The verifier for signatures from the log.
	SigVs []note.Verifier
	// The hash strategy that should be used in verifying consistency.
	Hasher hashers.LogHasher
	// An indicator of whether the log should be verified using consistency
	// proofs or compact ranges.
	UseCompact bool
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
			      chkpt BLOB,
			      range BLOB
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
// the parsed checkpoint and the note itself.
func (w *Witness) parse(chkptRaw []byte, logID string) (*log.Checkpoint, *note.Note, error) {
	logInfo, ok := w.Logs[logID]
	if !ok {
		return nil, nil, fmt.Errorf("log %q not found", logID)
	}
	n, err := note.Open(chkptRaw, note.VerifierList(logInfo.SigVs...))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to verify checkpoint: %v", err)
	}
	c := &log.Checkpoint{}
	if _, err := c.Unmarshal([]byte(n.Text)); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal new checkpoint: %v", err)
	}
	return c, n, nil
}

// GetLogs returns a list of all logs the witness is aware of.
func (w *Witness) GetLogs() ([]string, error) {
	rows, err := w.db.Query("SELECT logID FROM chkpts")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []string
	for rows.Next() {
		var logID string
		err := rows.Scan(&logID)
		if err != nil {
			return nil, err
		}
		logs = append(logs, logID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

// GetCheckpoint gets a checkpoint for a given log, which is consistent with all
// other checkpoints for the same log signed by this witness.
func (w *Witness) GetCheckpoint(logID string) ([]byte, error) {
	chkpt, _, err := w.getLatestChkptData(w.db.QueryRow, logID)
	if err != nil {
		return nil, err
	}
	return chkpt, nil
}

// Update updates the latest checkpoint if nextRaw is consistent with the current
// latest one for this log. It returns the latest cosigned checkpoint held by
// the witness, which is a signed version of nextRaw if the update was applied.
func (w *Witness) Update(ctx context.Context, logID string, nextRaw []byte, proof [][]byte) ([]byte, error) {
	// If we don't witness this log then no point in going further.
	logInfo, ok := w.Logs[logID]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "log %q not found", logID)
	}
	// Check the signatures on the raw checkpoint and parse it
	// into the log.Checkpoint format.
	next, nextNote, err := w.parse(nextRaw, logID)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse input checkpoint: %v", err)
	}
	// Optimistically sign the new checkpoint now.
	signed, err := w.signChkpt(nextNote)
	if err != nil {
		return nil, fmt.Errorf("couldn't sign input checkpoint: %v", err)
	}
	// Get the latest one for the log because we don't want consistency proofs
	// with respect to older checkpoints.  Bind this all in a transaction to
	// avoid race conditions when updating the database.
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't create db tx: %v", err)
	}
	// Get the latest checkpoint (if one exists) and compact range.
	prevRaw, rangeRaw, err := w.getLatestChkptData(tx.QueryRow, logID)
	if err != nil {
		// If there was nothing stored already then treat this new
		// checkpoint as trust-on-first-use (TOFU).
		if status.Code(err) == codes.NotFound {
			if err := w.setInitChkptData(tx, logID, next, signed, proof); err != nil {
				return nil, fmt.Errorf("couldn't set TOFU checkpoint: %v", err)
			}
			return signed, nil
		}
		return nil, fmt.Errorf("couldn't retrieve latest checkpoint: %w", err)
	}
	// Parse the raw retrieved checkpoint into the log.Checkpoint format.
	prev, _, err := w.parse(prevRaw, logID)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse stored checkpoint: %v", err)
	}
	if next.Origin != prev.Origin {
		return nil, fmt.Errorf("line 0 (origin log identifier) changed. Was %q, but attempting to set to %q", prev.Origin, next.Origin)
	}
	// Parse the compact range if we're using one.
	var prevRange log.Proof
	if logInfo.UseCompact {
		if err := prevRange.Unmarshal(rangeRaw); err != nil {
			return nil, fmt.Errorf("couldn't unmarshal proof: %v", err)
		}
	}
	if next.Size < prev.Size {
		// Complain if prev is bigger than next.
		return prevRaw, status.Errorf(codes.FailedPrecondition, "cannot prove consistency backwards (%d < %d)", next.Size, prev.Size)
	}
	if next.Size == prev.Size {
		if !bytes.Equal(next.Hash, prev.Hash) {
			return prevRaw, status.Errorf(codes.FailedPrecondition, "checkpoint for same size log with differing hash (got %x, have %x)", next.Hash, prev.Hash)
		}
		// If it's identical to the previous one do nothing.
		return prevRaw, nil
	}
	// The only remaining option is next.Size > prev.Size. This might be
	// valid so we use either plain consistency proofs or compact ranges to
	// verify, depending on the log.
	if logInfo.UseCompact {
		nextRange, err := verifyRange(next, prev, logInfo.Hasher, prevRange, proof)
		if err != nil {
			return prevRaw, status.Errorf(codes.FailedPrecondition, "failed to verify compact range: %v", err)
		}
		// If the proof is good store nextRaw and the new range.
		r := []byte(log.Proof(nextRange).Marshal())
		if err := w.setChkptData(tx, logID, signed, r); err != nil {
			return nil, fmt.Errorf("failed to store new checkpoint: %v", err)
		}
		return signed, nil
	}
	// If we're not using compact ranges then use consistency proofs.
	logV := logverifier.New(logInfo.Hasher)
	if err := logV.VerifyConsistencyProof(int64(prev.Size), int64(next.Size), prev.Hash, next.Hash, proof); err != nil {
		// Complain if the checkpoints aren't consistent.
		return prevRaw, status.Errorf(codes.FailedPrecondition, "failed to verify consistency proof: %v", err)
	}
	// If the consistency proof is good we store nextRaw.
	if err := w.setChkptData(tx, logID, signed, nil); err != nil {
		return nil, fmt.Errorf("failed to store new checkpoint: %v", err)
	}
	return signed, nil
}

// signChkpt adds the witness' signature to a checkpoint.
func (w *Witness) signChkpt(n *note.Note) ([]byte, error) {
	cosigned, err := note.Sign(n, w.Signer)
	if err != nil {
		return nil, fmt.Errorf("couldn't sign checkpoint: %v", err)
	}
	return cosigned, nil
}

// getLatestChkptData returns the raw stored data for the latest checkpoint and
// its associated compact range, if one exists, for a given log.
func (w *Witness) getLatestChkptData(queryRow func(query string, args ...interface{}) *sql.Row, logID string) ([]byte, []byte, error) {
	row := queryRow("SELECT chkpt, range FROM chkpts WHERE logID = ?", logID)
	if err := row.Err(); err != nil {
		return nil, nil, err
	}
	var chkpt []byte
	var pf []byte
	if err := row.Scan(&chkpt, &pf); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, status.Errorf(codes.NotFound, "no checkpoint for log %q", logID)
		}
		return nil, nil, err
	}
	return chkpt, pf, nil
}

// verifyRange verifies the new checkpoint against the stored and given compact
// range and outputs the updated compact range if verification succeeds.
func verifyRange(next *log.Checkpoint, prev *log.Checkpoint, h hashers.LogHasher, rngRaw [][]byte, deltaRaw [][]byte) ([][]byte, error) {
	rf := compact.RangeFactory{Hash: h.HashChildren}
	rng, err := rf.NewRange(0, prev.Size, rngRaw)
	if err != nil {
		return nil, fmt.Errorf("can't form current compact range: %v", err)
	}
	// As a sanity check, make sure the stored checkpoint and range are consistent.
	if err := verifyRangeHash(prev.Hash, rng); err != nil {
		return nil, fmt.Errorf("old root hash doesn't verify: %v", err)
	}
	delta, err := rf.NewRange(prev.Size, next.Size, deltaRaw)
	if err != nil {
		return nil, fmt.Errorf("can't form delta compact range: %v", err)
	}
	// Merge the delta range into the existing one and compare root hashes.
	rng.AppendRange(delta, nil)
	if err := verifyRangeHash(next.Hash, rng); err != nil {
		return nil, fmt.Errorf("new root hash doesn't verify: %v", err)
	}
	return rng.Hashes(), nil
}

// verifyRangeHash computes the root hash of the compact range and compares it
// against the one given as input, returning an error if they aren't equal.
func verifyRangeHash(rootHash []byte, rng *compact.Range) error {
	h, err := rng.GetRootHash(nil)
	if err != nil {
		return fmt.Errorf("can't get root hash for range: %v", err)
	}
	if !bytes.Equal(rootHash, h) {
		return fmt.Errorf("hashes aren't equal (got %x, given %x)", h, rootHash)
	}
	return nil
}

// setChkptData writes the checkpoint and any associated data (a compact range)
// to the database for a given log.
func (w *Witness) setChkptData(tx *sql.Tx, logID string, c []byte, rng []byte) error {
	tx.Exec(`INSERT INTO chkpts (logID, chkpt, range) VALUES (?, ?, ?)
		 ON CONFLICT(logID) DO
		 UPDATE SET chkpt=excluded.chkpt, range=excluded.range`,
		logID, c, rng)
	return tx.Commit()
}

// setInitChkptData stores the data for an initial checkpoint and, if using one,
// its associated compact range.
func (w *Witness) setInitChkptData(tx *sql.Tx, logID string, c *log.Checkpoint, cRaw []byte, rngRaw [][]byte) error {
	logInfo := w.Logs[logID]
	// If we're using compact ranges then store the initial range, assuming
	// it matches the initial checkpoint.
	if logInfo.UseCompact {
		rf := compact.RangeFactory{Hash: logInfo.Hasher.HashChildren}
		rng, err := rf.NewRange(0, c.Size, rngRaw)
		if err != nil {
			return fmt.Errorf("can't form compact range: %v", err)
		}
		if err := verifyRangeHash(c.Hash, rng); err != nil {
			return fmt.Errorf("input root hash doesn't verify: %v", err)
		}
		r := []byte(log.Proof(rngRaw).Marshal())
		return w.setChkptData(tx, logID, cRaw, r)
	}
	// If we're not using compact ranges no need to store one.
	return w.setChkptData(tx, logID, cRaw, nil)
}
