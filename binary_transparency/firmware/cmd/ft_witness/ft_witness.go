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

// This package is the entrypoint for the Firmware Transparency witness server.
package main

import (
	"context"
	"flag"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/cmd/ft_witness/impl"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/crypto"
	"golang.org/x/mod/sumdb/note"
)

var (
	listenAddr   = flag.String("listen", ":8020", "address:port to listen for requests on")
	wsFile       = flag.String("ws_db_file", "", "Path to a file to be used as simple file storage for checkpoint, e.g. /tmp/witness.db")
	ftLogURL     = flag.String("ftlog", "http://localhost:8000", "Base URL of FT Log server")
	pollInterval = flag.Duration("poll_interval", 5*time.Second, "Duration to wait between polling FT Log for new entries")
)

func main() {
	flag.Parse()

	testVerifier, _ := note.NewVerifier(crypto.TestFTPersonalityPub)

	ctx := context.Background()
	if err := impl.Main(ctx, impl.WitnessOpts{
		ListenAddr:       *listenAddr,
		WSFile:           *wsFile,
		FtLogURL:         *ftLogURL,
		FtLogSigVerifier: testVerifier,
		PollInterval:     *pollInterval,
	}); err != nil {
		glog.Exit(err.Error())
	}
}
