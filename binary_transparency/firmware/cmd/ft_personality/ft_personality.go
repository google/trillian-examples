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

// This package is the entrypoint for the Firmware Transparency personality server.
// This requires a Trillian instance to be reachable via gRPC and a tree to have
// been provisioned. See the README in the root of this project for instructions.
package main

import (
	"context"
	"flag"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/cmd/ft_personality/impl"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/crypto"
	"golang.org/x/mod/sumdb/note"
)

var (
	listenAddr = flag.String("listen", ":8000", "address:port to listen for requests on")

	connectTimeout = flag.Duration("connect_timeout", time.Second, "the timeout for connecting to the backend")
	trillianAddr   = flag.String("trillian", ":8090", "address:port of Trillian Log gRPC service")

	casDBFile = flag.String("cas_db_file", "", "Path to a file to be used as sqlite3 storage for images, e.g. /tmp/ft.db")

	sthRefresh = flag.Duration("sth_refresh_interval", 5*time.Second, "how often to fetch the latest log root from Trillian")
)

func main() {
	flag.Parse()

	signer, err := note.NewSigner(crypto.TestFTPersonalityPriv)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	if err := impl.Main(ctx, impl.PersonalityOpts{
		ListenAddr:     *listenAddr,
		ConnectTimeout: *connectTimeout,
		TrillianAddr:   *trillianAddr,
		CASFile:        *casDBFile,
		STHRefresh:     *sthRefresh,
		Signer:         signer,
	}); err != nil {
		glog.Exit(err.Error())
	}
}
