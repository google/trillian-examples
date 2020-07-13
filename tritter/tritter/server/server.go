// Copyright 2019 Google LLC
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
	"net"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/tritter/tritter"
	"google.golang.org/grpc"
)

var (
	listenAddr = flag.String("listen_addr", "localhost:50051", "the address the server is listening on")
)

// server is used to implement TritterServer.
type server struct {
	tritter.UnimplementedTritterServer
}

// Send implements TritterServer.Send.
func (s *server) Send(ctx context.Context, in *tritter.SendRequest) (*tritter.SendResponse, error) {
	glog.Infof("Send: %v", in.GetMessage())
	return &tritter.SendResponse{}, nil
}

func main() {
	flag.Parse()
	lis, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		glog.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	tritter.RegisterTritterServer(s, &server{})
	glog.Infof("Serving tritter on %v", *listenAddr)
	if err := s.Serve(lis); err != nil {
		glog.Fatalf("failed to serve: %v", err)
	}
}
