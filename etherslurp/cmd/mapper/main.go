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

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian-examples/etherslurp/mapper"
	"google.golang.org/grpc"
)

var (
	trillianLog = flag.String("trillian_log", "", "URL of the Trillian Log RPC server.")
	trillianMap = flag.String("trillian_map", "", "URL of the Trillian Map RPC server.")
	logID       = flag.Int64("log_id", 0, "Trillian LogID to populate.")
	mapID       = flag.Int64("map_id", 0, "Trillian MapID to populate.")
	from        = flag.Int64("from", 0, "Block to start at.")
)

func main() {
	flag.Parse()
	ctx := context.Background()

	if *logID == 0 {
		glog.Exitf("LogID is set to zero, I don't believe you! Set --log_id")
	}
	if *mapID == 0 {
		glog.Exitf("MapID is set to zero, I don't believe you! Set --map_id")
	}

	tlc, err := grpc.Dial(*trillianLog, grpc.WithInsecure())
	if err != nil {
		glog.Exitf("Failed to dial Trillian Log: %v", err)
	}

	tmc, err := grpc.Dial(*trillianMap, grpc.WithInsecure())
	if err != nil {
		//		glog.Exitf("Failed to dial Trillian Map: %v", err)
	}

	m := mapper.New(trillian.NewTrillianLogClient(tlc), *logID, trillian.NewTrillianMapClient(tmc), *mapID)
	m.Map(ctx, *from)
}
