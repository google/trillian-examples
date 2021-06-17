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

// ctverify checks that leaf data downloaded by ctclone is committed to by a checkpoint.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io/ioutil"
	"os"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/clone/internal/verify"
	"github.com/google/trillian-examples/clone/logdb"
	"github.com/google/trillian/merkle/rfc6962/hasher"

	_ "github.com/go-sql-driver/mysql"
)

var (
	mysqlURI = flag.String("mysql_uri", "", "URL of a MySQL database to clone the log into. The DB should contain only one log.")
)

func main() {
	flag.Parse()

	if len(*mysqlURI) == 0 {
		glog.Exit("Missing required parameter 'mysql_uri'")
	}
	db, err := logdb.NewDatabase(*mysqlURI)
	if err != nil {
		glog.Exitf("Failed to connect to database: %q", err)
	}

	bs, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		glog.Exitf("Failed to read checkpoint from stdin: %q", err)
	}
	cp := CTCheckpointResponse{}
	if err := json.Unmarshal(bs, &cp); err != nil {
		glog.Exitf("Failed to read checkpoint from stdin: %q", err)
	}
	glog.Infof("Parsed checkpoint with size %d and root hash %x. Calculating root hash for local data...", cp.TreeSize, cp.RootHash)

	h := hasher.DefaultHasher
	lh := func(_ uint64, preimage []byte) []byte {
		return h.HashLeaf(preimage)
	}
	v := verify.NewLogVerifier(db.StreamLeaves, lh, h.HashChildren)
	root, err := v.MerkleRoot(cp.TreeSize)
	if err != nil {
		glog.Exitf("Failed to compute root: %q", err)
	}
	if bytes.Equal(cp.RootHash, root) {
		glog.Infof("Got matching roots for tree size %d: %x", cp.TreeSize, root)
	} else {
		glog.Exitf("Computed root %x != provided checkpoint %x for tree size %d", root, cp.RootHash, cp.TreeSize)
	}
}

// CTCheckpointResponse mirrors the RFC6962 STH format for `get-sth` to allow the
// data to be easy unmarshalled from the JSON response.
type CTCheckpointResponse struct {
	TreeSize  uint64 `json:"tree_size"`
	Timestamp uint64 `json:"timestamp"`
	RootHash  []byte `json:"sha256_root_hash"`
	Sig       []byte `json:"tree_head_signature"`
}
