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

// This package is the entrypoint for the Firmware Transparency map server.
// This requires a Trillian instance to be reachable via gRPC and a tree to have
// been provisioned. See the README in the root of this project for instructions.
package main

import (
	"context"
	"flag"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/cmd/ftmapserver/impl"
)

var (
	listenAddr = flag.String("listen", ":8001", "address:port to listen for requests on")
	mapDBAddr  = flag.String("map_db", "", "Connection path for map database")
)

func main() {
	flag.Parse()

	ctx := context.Background()
	if err := impl.Main(ctx, impl.MapServerOpts{
		ListenAddr: *listenAddr,
		MapDBAddr:  *mapDBAddr,
	}); err != nil {
		glog.Exit(err.Error())
	}
}
