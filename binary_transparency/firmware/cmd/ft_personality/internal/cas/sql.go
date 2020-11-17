// Copyright 2020 Google LLC. All Rights Reserved.
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

// Package cas contains a Content Addressable Store.
package cas

import (
	"database/sql"
)

// BinaryStorage is a CAS intended for storing binary images keyed by their hash string
// that uses a SQL Database as its backing store.
type BinaryStorage struct {
	db *sql.DB
}

// NewBinaryStorage creates a new CAS that uses the given DB as a backend.
// The DB will be initialized if needed.
func NewBinaryStorage(db *sql.DB) (*BinaryStorage, error) {
	cas := &BinaryStorage{
		db: db,
	}
	return cas, cas.init()
}

// init creates the database tables if needed.
func (bs *BinaryStorage) init() error {
	_, err := bs.db.Exec("CREATE TABLE IF NOT EXISTS images (key BLOB PRIMARY KEY, data BLOB)")
	return err
}

// Store stores a binary image under the given key (which should be a hash of its data).
// If there was an existing value under the key then it will not be updated.
// TODO(mhutchinson): Consider validating that the image is exactly the same if the key
// already exists. This should only happen if the hash function isn't strong.
func (bs *BinaryStorage) Store(key, image []byte) error {
	_, err := bs.db.Exec("INSERT OR IGNORE INTO images (key, data) VALUES (?, ?)", key, image)
	return err
}

// Retrieve gets a binary image that was previously stored.
func (bs *BinaryStorage) Retrieve(key []byte) ([]byte, error) {
	var res []byte
	row := bs.db.QueryRow("SELECT data FROM images WHERE key=?", key)
	if row.Err() != nil {
		return nil, row.Err()
	}
	row.Scan(&res)
	return res, nil
}
