// Copyright 2022 Google LLC. All Rights Reserved.
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

// Package sql provides log state persistence backed by a SQL database.
package sql

import (
	"database/sql"
	"fmt"

	"github.com/google/trillian-examples/witness/golang/internal/persistence"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NewPersistence returns a persistence object that is backed by the SQL database.
func NewPersistence(db *sql.DB) persistence.LogStatePersistence {
	return &sqlLogPersistence{
		db: db,
	}
}

type sqlLogPersistence struct {
	db *sql.DB
}

func (p *sqlLogPersistence) Init() error {
	_, err := p.db.Exec(`CREATE TABLE IF NOT EXISTS chkpts (
		logID BLOB PRIMARY KEY,
		chkpt BLOB,
		range BLOB
		)`)
	return err
}

func (p *sqlLogPersistence) Logs() ([]string, error) {
	rows, err := p.db.Query("SELECT logID FROM chkpts")
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

func (p *sqlLogPersistence) ReadOps(logID string) (persistence.LogStateReadOps, error) {
	return &reader{
		logID: logID,
		db:    p.db,
	}, nil
}

func (p *sqlLogPersistence) WriteOps(logID string) (persistence.LogStateWriteOps, error) {
	tx, err := p.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("tx.Begin(): %v", err)
	}
	return &writer{
		logID: logID,
		tx:    tx,
	}, nil
}

type reader struct {
	logID string
	db    *sql.DB
}

func (r *reader) GetLatestCheckpoint() ([]byte, []byte, error) {
	return getLatestCheckpoint(r.db.QueryRow, r.logID)
}

type writer struct {
	logID string
	tx    *sql.Tx
}

func (w *writer) GetLatestCheckpoint() ([]byte, []byte, error) {
	return getLatestCheckpoint(w.tx.QueryRow, w.logID)
}

func (w *writer) SetCheckpoint(c []byte, rng []byte) error {
	_, err := w.tx.Exec(`INSERT OR REPLACE INTO chkpts (logID, chkpt, range) VALUES (?, ?, ?)`, w.logID, c, rng)
	if err != nil {
		return fmt.Errorf("Exec(): %v", err)
	}
	return w.tx.Commit()
}

func (w *writer) Close() {
	w.tx.Rollback()
}

func getLatestCheckpoint(queryRow func(query string, args ...interface{}) *sql.Row, logID string) ([]byte, []byte, error) {
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
