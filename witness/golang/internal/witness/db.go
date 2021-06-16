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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Database reads from and writes checkpoints to persistent storage.
type Database struct {
	db *sql.DB
}

// NewDatabase creates a new database, initializing it if needed.
func NewDatabase(db *sql.DB) (*Database, error) {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS chkpts (key BLOB, size INT, raw BLOB, PRIMARY KEY (key, size))")
	if err != nil {
		return nil, err
	}
	return &Database{db: db}, nil
}

// GetLatest reads the latest checkpoint written to the DB for a given log.
func (d *Database) GetLatest(logID string) (*Chkpt, error) {
	return d.getLatestChkpt(d.db.QueryRow, logID)
}

func (d *Database) getLatestChkpt(queryRow func(query string, args ...interface{}) *sql.Row, logID string) (*Chkpt, error) {
	var maxChkpt Chkpt
	row := queryRow("SELECT raw, size FROM chkpts WHERE key = ? ORDER BY size DESC LIMIT 1", logID)
	if err := row.Err(); err != nil {
		return nil, err
	}
	if err := row.Scan(&maxChkpt.Raw, &maxChkpt.Size); err != nil {
		if err == sql.ErrNoRows {
			return nil, status.Errorf(codes.NotFound, "log %q not found", logID)
		}
		return nil, err
	}
	return &maxChkpt, nil
}

// SetCheckpoint writes the checkpoint to the DB for a given log, assuming
// that the latest size is still what the caller thought it was.
func (d *Database) SetCheckpoint(ctx context.Context, logID string, latestSize uint64, c *Chkpt) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("BeginTx: %w", err)
	}
	realLatest, err := d.getLatestChkpt(tx.QueryRow, logID)
	if latestSize > 0 {
		// If latest is non-zero check it's the same as the one in the DB.
		if err != nil {
			return fmt.Errorf("GetLatest: %w", err)
		}
		if latestSize != realLatest.Size {
			return fmt.Errorf("latest checkpoint changed in the meantime")
		}
	} else {
		// Otherwise check that there really isn't anything there.
		if err == nil {
			return fmt.Errorf("got latest=nil but there was a stored checkpoint")
		}
		if status.Code(err) != codes.NotFound {
			return fmt.Errorf("GetLatest: %w", err)
		}
	}
	tx.Exec("INSERT OR IGNORE INTO chkpts (key, size, raw) VALUES (?, ?, ?)", logID, c.Size, c.Raw)
	return tx.Commit()
}
