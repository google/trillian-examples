// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package mysql provides a MySQL based implementation of incident management.
package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/gossip/incident"
)

type mysqlReporter struct {
	db     *sql.DB
	stmt   *sql.Stmt
	source string
}

// NewMySQLReporter builds an incident.Reporter instance that records incidents
// in a MySQL database, all of which will be marked as emanating from the given
// source.
func NewMySQLReporter(ctx context.Context, db *sql.DB, source string) (incident.Reporter, error) {
	stmt, err := db.PrepareContext(ctx, "INSERT INTO Incidents(Timestamp, Source, BaseURL, Summary, IsViolation, FullURL, Details) VALUES (?, ?, ?, ?, ?, ?, ?);")
	if err != nil {
		return nil, fmt.Errorf("failed to prepare context for %q: %v", source, err)
	}
	return &mysqlReporter{db: db, source: source, stmt: stmt}, nil
}

// LogUpdate records an incident with the given details.
func (m *mysqlReporter) LogUpdate(ctx context.Context, baseURL, summary, fullURL, details string) {
	now := time.Now()
	glog.Errorf("[%s] %s: %s (url=%s)\n  %s", now, baseURL, summary, fullURL, details)
	if _, err := m.stmt.ExecContext(ctx, now, m.source, baseURL, summary, false /* isViolation */, fullURL, details); err != nil {
		glog.Errorf("failed to insert incident for %q: %v", m.source, err)
	}
}

// LogViolation records an incident with the given details.
func (m *mysqlReporter) LogViolation(ctx context.Context, baseURL, summary, fullURL, details string) {
	now := time.Now()
	glog.Infof("[%s] %s: %s (url=%s)\n  %s", now, baseURL, summary, fullURL, details)
	if _, err := m.stmt.ExecContext(ctx, now, m.source, baseURL, summary, true /* isViolation */, fullURL, details); err != nil {
		glog.Errorf("failed to insert incident for %q: %v", m.source, err)
	}
}

// LogUpdatef records an incident with the given details and formatting.
func (m *mysqlReporter) LogUpdatef(ctx context.Context, baseURL, summary, fullURL, detailsFmt string, args ...interface{}) {
	details := fmt.Sprintf(detailsFmt, args...)
	m.LogUpdate(ctx, baseURL, summary, fullURL, details)
}

// LogUpdatef records an incident with the given details and formatting.
func (m *mysqlReporter) LogViolationf(ctx context.Context, baseURL, summary, fullURL, detailsFmt string, args ...interface{}) {
	details := fmt.Sprintf(detailsFmt, args...)
	m.LogViolation(ctx, baseURL, summary, fullURL, details)
}
