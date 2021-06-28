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
package main

import (
	"context"
	"flag"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/witness/cmd/internal/witness"
)

var (
	listenAddr = flag.String("listen", ":8000", "address:port to listen for requests on")
	dbFile = flag.String("db_file", "", "Path to a file to be used as sqlite3 storage for checkpoints, e.g. /tmp/chkpts.db")
)

func main() {
	flag.Parse()

	glog.Infof("Connecting to local DB at %q", *dbFile)
	db, err := sql.Open("sqlite3", *dbFile)
	if err != nil {
		panic(err)
	}

	signer, err := note.NewSigner(crypto.TestWitnessPriv)
	if err != nil {
		panic(err)
	}
	h := hasher.DefaultHasher
	logV, err := note.NewVerifier(logPK)
	if err != nil {
		panic(err)
	}
	sigVs := []note.Verifier{logV}
	log := LogInfo{
		SigVs: sigVs,
		LogV:  logverifier.New(h),
	}
	logs := map[string]LogInfo{logID: log}

	ctx := context.Background()
	if err := impl.Main(ctx, witness.Opts{
		Database:	db,
		Signer:		signer,
		KnownLogs:	logs,
	}); err != nil {
		glog.Exitf("Error running witness: %q", err)
	}
    }
