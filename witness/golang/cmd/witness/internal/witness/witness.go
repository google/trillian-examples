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

// LogInfo contains the information needed to verify the signatures of a given
// log, the hash strategy that should be used for verifying its consistency, and
// a boolean indicating if the log should be verified using consistency proofs or
// compact ranges.
type LogInfo struct {
	SigVs      []note.Verifier
	Hasher     hashers.LogHasher
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
// other checkpoints for the same log signed by this witness.  It also signs it
// under the witness' key.
func (w *Witness) GetCheckpoint(logID string) ([]byte, error) {
	chkpt, _, err := w.getLatestChkptData(w.db.QueryRow, logID)
	if err != nil {
		return nil, err
	}
	// Add the witness' signature to the checkpoint.
	logInfo, ok := w.Logs[logID]
	if !ok {
		return nil, fmt.Errorf("log %q not found", logID)
	}
	n, err := note.Open(chkpt, note.VerifierList(logInfo.SigVs...))
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
	chkptNew, err := w.parse(chkptRaw, logID)
	if err != nil {
		return 0, err
	}
	// Get the latest one for the log because we don't want consistency proofs
	// with respect to older checkpoints.  Bind this all in a transaction to
	// avoid race conditions when updating the database.
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("couldn't create tx: %v", err)
	}
	// Get the latest checkpoint and compact range (if one exists).
	latestRaw, rangeRaw, err := w.getLatestChkptData(tx.QueryRow, logID)
	if err != nil {
		// If there was nothing stored already then treat this new
		// checkpoint as trust-on-first-use.
		if status.Code(err) == codes.NotFound {
			// If we're using compact ranges then store the input
			// proof as our initial range, assuming it matches the
			// input checkpoint.
			if logInfo.UseCompact {
				rf := compact.RangeFactory{Hash: logInfo.Hasher.HashChildren}
				rng, err := rf.NewRange(0, chkptNew.Size, proof)
				if err != nil {
					return 0, fmt.Errorf("can't form compact range: %v", err)
				}
				if err := w.verifyRangeHash(chkptNew.Hash, rng); err != nil {
					return 0, fmt.Errorf("input root hash doesn't verify: %v", err)
				}
				rangeRaw := []byte(log.Proof(proof).Marshal())
				return 0, w.setChkptData(tx, logID, chkptRaw, rangeRaw)
			}
			return 0, w.setChkptData(tx, logID, chkptRaw, nil)
		}
		return 0, fmt.Errorf("Update: %w", err)
	}
	// Parse the raw checkpoint into the log.Checkpoint format.
	chkptOld, err := w.parse(latestRaw, logID)
	if err != nil {
		return 0, fmt.Errorf("couldn't parse stored checkpoint: %v", err)
	}
	// Parse the compact range if we're using one.
	var curRange log.Proof
	if logInfo.UseCompact {
		if err := curRange.Unmarshal(rangeRaw); err != nil {
			return 0, fmt.Errorf("couldn't unmarshal proof: %v", err)
		}
	}
	switch {
	case chkptNew.Size < chkptOld.Size:
		// Complain if old checkpoint is bigger than new one.
		return chkptOld.Size, fmt.Errorf("cannot prove consistency backwards (%d < %d)", chkptNew.Size, chkptOld.Size)
	case chkptNew.Size > chkptOld.Size:
		// Potentially a valid update, use either plain consistency proofs
		// or compact ranges to verify, depending on the log.
		switch {
		case logInfo.UseCompact:
			newRange, err := w.verifyRange(chkptNew, chkptOld, logInfo.Hasher, curRange, proof)
			if err != nil {
				return chkptOld.Size, fmt.Errorf("failed to verify compact range: %v", err)
			}
			// If the proof is good store chkptRaw and the new range.
			rangeRaw := []byte(log.Proof(newRange).Marshal())
			return chkptOld.Size, w.setChkptData(tx, logID, chkptRaw, rangeRaw)
		// Plain consistency proofs are the default.
		default:
			logV := logverifier.New(logInfo.Hasher)
			if err := logV.VerifyConsistencyProof(int64(chkptOld.Size), int64(chkptNew.Size), chkptOld.Hash, chkptNew.Hash, proof); err != nil {
				// Complain if the checkpoints aren't consistent.
				return chkptOld.Size, fmt.Errorf("failed to verify consistency proof: %v", err)
			}
			// If the consistency proof is good we store chkptRaw.
			return chkptOld.Size, w.setChkptData(tx, logID, chkptRaw, nil)
		}
	case chkptNew.Size == chkptOld.Size:
		if !bytes.Equal(chkptNew.Hash, chkptOld.Hash) {
			return chkptOld.Size, fmt.Errorf("checkpoint for same size log with differing hash (got %x, have %x)", chkptNew.Hash, chkptOld.Hash)
		}
		// If it's identical to the latest do nothing.
		return chkptOld.Size, nil
	default:
		panic("unreachable")
	}
}

// getLatestChkptData returns the raw stored data for the latest checkpoint and
// its associated compact range, if one exists, for a given log.
func (w *Witness) getLatestChkptData(queryRow func(query string, args ...interface{}) *sql.Row, logID string) ([]byte, []byte, error) {
	row := queryRow("SELECT chkpt, range FROM chkpts WHERE logID = ?", logID)
	if err := row.Err(); err != nil {
		return nil, nil, err
	}
	var rawChkpt []byte
	var rawPf []byte
	if err := row.Scan(&rawChkpt, &rawPf); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, status.Errorf(codes.NotFound, "no checkpoint for log %q", logID)
		}
		return nil, nil, err
	}
	return rawChkpt, rawPf, nil
}

// verifyRange verifies the new checkpoint against the stored and given compact
// range and outputs the new compact range if verification succeeds.
func (w *Witness) verifyRange(chkptNew *log.Checkpoint, chkptOld *log.Checkpoint, h hashers.LogHasher, curRangeRaw [][]byte, newRangeRaw [][]byte) ([][]byte, error) {
	rf := compact.RangeFactory{Hash: h.HashChildren}
	curRange, err := rf.NewRange(0, chkptOld.Size, curRangeRaw)
	if err != nil {
		return nil, fmt.Errorf("can't form current compact range: %v", err)
	}
	// As a sanity check, make sure the old checkpoint is consistent with
	// the current range.
	if err := w.verifyRangeHash(chkptOld.Hash, curRange); err != nil {
		return nil, fmt.Errorf("old root hash doesn't verify: %v", err)
	}
	newRange, err := rf.NewRange(chkptOld.Size, chkptNew.Size, newRangeRaw)
	if err != nil {
		return nil, fmt.Errorf("can't form new compact range: %v", err)
	}
	// Merge the new range into the existing one and compare root hashes.
	curRange.AppendRange(newRange, nil)
	if err := w.verifyRangeHash(chkptNew.Hash, curRange); err != nil {
		return nil, fmt.Errorf("new root hash doesn't verify: %v", err)
	}
	return curRange.Hashes(), nil
}

// verifyRangeHash computes the root hash of the compact range and compares it
// against the one given as input, returning an error if they aren't equal.
func (w *Witness) verifyRangeHash(rootHash []byte, rng *compact.Range) error {
	h, err := rng.GetRootHash(nil)
	if err != nil {
		return fmt.Errorf("can't get root hash for range: %v", err)
	}
	if !bytes.Equal(rootHash, h) {
		return fmt.Errorf("old hashes aren't equal (got %x, given %x)", h, rootHash)
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
