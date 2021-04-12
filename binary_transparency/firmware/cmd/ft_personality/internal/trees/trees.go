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

// Package trees contains the personality tree configuration.
package trees

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/client"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/crypto/sigpb"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

// TreeStorage allows access to the configuration.
type TreeStorage struct {
	db *sql.DB
}

// NewTreeStorage sets up a TreeStorage using the given DB.
func NewTreeStorage(db *sql.DB) *TreeStorage {
	return &TreeStorage{
		db: db,
	}
}

// EnsureTree gets the tree for this personality, creating it if necessary.
// Takes a connection to Trillian to use for initialization.
// This is only safe in a single-replica deployment. In a real production setup
// this provisioning would likely be done by a human ahead of time, or if this
// style of automatic deployment was required then some kind of locking would
// be required to ensure that only one log was created and used by all frontends.
func (s *TreeStorage) EnsureTree(ctx context.Context, conn grpc.ClientConnInterface) (*trillian.Tree, error) {
	if err := s.init(); err != nil {
		return nil, err
	}
	res, err := s.getTree()
	if err == nil {
		return res, nil
	}
	if err == sql.ErrNoRows {
		tree, err := s.createTree(ctx, conn)
		if err != nil {
			return nil, err
		}
		return tree, s.setTree(tree)
	}
	return nil, err
}

func (s *TreeStorage) createTree(ctx context.Context, conn grpc.ClientConnInterface) (*trillian.Tree, error) {
	// N.B. Using the admin interface from the personality is not good practice for
	// a production system. This simply allows a convenient way of getting the tree
	// for the sake of getting the FT demo up and running.

	ctr := &trillian.CreateTreeRequest{
		Tree: &trillian.Tree{
			TreeState:          trillian.TreeState_ACTIVE,
			TreeType:           trillian.TreeType_LOG,
			HashStrategy:       trillian.HashStrategy_RFC6962_SHA256,
			HashAlgorithm:      sigpb.DigitallySigned_SHA256,
			SignatureAlgorithm: sigpb.DigitallySigned_ECDSA,
			DisplayName:        "ft",
			Description:        "binary transparency log",
			MaxRootDuration:    durationpb.New(time.Hour),
		},
		KeySpec: &keyspb.Specification{
			Params: &keyspb.Specification_EcdsaParams{
				EcdsaParams: &keyspb.Specification_ECDSA{},
			},
		},
	}

	adminClient := trillian.NewTrillianAdminClient(conn)
	logClient := trillian.NewTrillianLogClient(conn)

	return client.CreateAndInitTree(ctx, ctr, adminClient, logClient)
}

func (s *TreeStorage) getTree() (*trillian.Tree, error) {
	var raw []byte
	if err := s.db.QueryRow("SELECT config FROM trees WHERE key = 'ft'").Scan(&raw); err != nil {
		return nil, err
	}

	var res *trillian.Tree
	if err := proto.Unmarshal(raw, res); err != nil {
		return nil, err
	}
	return res, nil
}

func (s *TreeStorage) setTree(tree *trillian.Tree) error {
	bs, err := proto.Marshal(tree)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT INTO trees (key, config) VALUES (?, ?)", "ft", bs)
	if err != nil {
		return fmt.Errorf("failed to set tree: %w", err)
	}
	return nil
}

func (s *TreeStorage) init() error {
	_, err := s.db.Exec("CREATE TABLE IF NOT EXISTS trees (key BLOB PRIMARY KEY, config BLOB)")
	return err
}
