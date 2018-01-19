// Copyright 2018 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"flag"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/golang/glog"
	"github.com/google/trillian"
	"google.golang.org/grpc"

	"trillian-examples-ethereum/follower"
)

var (
	geth        = flag.String("geth", "", "URL of the geth RPC server.")
	trillianLog = flag.String("trillian_log", "", "URL of the Trillian Log RPC server.")
	logID       = flag.Int64("log_id", 0, "Trillian LogID to populate.")
)

func main() {
	flag.Parse()
	ctx := context.Background()

	if *logID == 0 {
		glog.Exitf("LogID is set to zero, I don't believe you! Set --log_id")
	}

	gc, err := ethclient.Dial(*geth)
	if err != nil {
		glog.Exitf("Failed to dial geth: %v", err)
	}

	tc, err := grpc.Dial(*trillianLog, grpc.WithInsecure())
	if err != nil {
		glog.Exitf("Failed to dial Trillian Log: %v", err)
	}

	f := follower.New(gc, trillian.NewTrillianLogClient(tc), *logID, follower.FollowerOpts{})
	f.Follow(ctx)
}
