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

package witness

import (
	"context"
	"database/sql"
	"fmt"
)

// Database simply gets and puts things into persistent storage.
type Database struct {
	db *sql.DB
}

// NewDatabase creates a new database, initializing it if needed.
func NewDatabase(db *sql.DB) (*Database, error) {
	d := &Database{
		db: db,
	}
	return d, d.init()
}

func (d *Database) init() error {
	_, err := d.db.Exec("CREATE TABLE IF NOT EXISTS chkpts (key BLOB, size INT, raw BLOB, PRIMARY KEY (key, size))")
	return err
}

type querier interface {
	QueryRow(query string, args ...interface{}) *sql.Row
}

// GetLatest reads the latest checkpoint written to the DB for a given log.
func (d *Database) GetLatest(logPK string) (*Chkpt, error) {
	return d.getLatestChkpt(d.db, logPK)
}

func (d *Database) getLatestChkpt(q querier, logPK string) (*Chkpt, error) {
	var maxChkpt Chkpt
	row := q.QueryRow("SELECT raw, size FROM chkpts WHERE key = ? ORDER BY size DESC LIMIT 1", logPK)
	if err := row.Err(); err != nil {
		return nil, err
	}
	if err := row.Scan(&maxChkpt.Raw, &maxChkpt.Size); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("unknown public key: %q: %w", logPK, err)
		}
		return nil, err
	}
	return &maxChkpt, nil
}

// SetCheckpoint writes the checkpoint to the DB for a given logPK, assuming
// that the latest checkpoint is still what the caller thought it was.
func (d *Database) SetCheckpoint(logPK string, latest, c *Chkpt) error {
	tx, err := d.db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("BeginTx: %v", err)
	}
	if latest != nil {
		realLatest, err := d.getLatestChkpt(tx, logPK)
		if err != nil {
			return fmt.Errorf("GetLatest: %v", err)
		}
		if latest.Size != realLatest.Size {
			return fmt.Errorf("latest checkpoint changed in the meantime")
		}
	}
	tx.Exec("INSERT OR IGNORE INTO chkpts (key, size, raw) VALUES (?, ?, ?)", logPK, c.Size, c.Raw)
	return tx.Commit()
}
