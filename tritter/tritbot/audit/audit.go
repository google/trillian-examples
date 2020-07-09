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

// Checks that the log is append-only.
package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian-examples/tritter/tritbot/log"
	tc "github.com/google/trillian/client"
	"github.com/google/trillian/types"
	tt "github.com/google/trillian/types"
	"google.golang.org/grpc"
)

var (
	loggerAddr       = flag.String("logger_addr", "localhost:50053", "the address of the trillian logger personality")
	connectTimeout   = flag.Duration("connect_timeout", time.Second, "the timeout for connecting to the server")
	fetchRootTimeout = flag.Duration("fetch_root_timeout", 2*time.Second, "the timeout for fetching the latest log root")

	pollInterval = flag.Duration("poll_interval", 5*time.Second, "how often to audit the log")
)

type auditor struct {
	timeout time.Duration

	log log.LoggerClient
	v   *tc.LogVerifier
	con *grpc.ClientConn

	trustedRoot tt.LogRootV1
}

func new(ctx context.Context) *auditor {
	// Set up a connection to the Logger server.
	lCon, err := grpc.DialContext(ctx, *loggerAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		glog.Fatalf("could not connect to logger on %v: %v", *loggerAddr, err)
	}
	v, err := log.TreeVerifier()
	if err != nil {
		glog.Fatalf("could not create tree verifier: %v", err)
	}

	return &auditor{
		timeout: *fetchRootTimeout,
		log:     log.NewLoggerClient(lCon),
		v:       v,
		con:     lCon,
	}
}

func (a *auditor) checkLatest(ctx context.Context) error {
	// 1. Get latest root & check consistency.
	newRoot, err := a.getLatest(ctx)
	if err != nil {
		return fmt.Errorf("failed to get latest valid log root: %v", err)
	}

	if newRoot.Revision > a.trustedRoot.Revision {
		// 2. Check that all leaves since the last audited revision are valid.
		for i := a.trustedRoot.TreeSize; i < newRoot.TreeSize; i++ {
			bs, err := a.getIndex(ctx, newRoot, int64(i))
			if err != nil {
				return fmt.Errorf("failed to get & verify leaf at index %d in revision %d: %v", i, newRoot.Revision, err)
			}

			var msg log.InternalMessage
			if err := proto.UnmarshalText(string(bs), &msg); err != nil {
				return fmt.Errorf("failed to unmarshal verified bytes at index %d in revision %d: %v", i, newRoot.Revision, err)
			}
			glog.V(2).Infof("Confirmed data at index %d: %v", i, msg)
		}

		// 3. Update the trusted root to latest audited value.
		a.trustedRoot = *newRoot
		glog.Infof("updated trusted root to revision=%d with size=%d", newRoot.Revision, newRoot.TreeSize)
	}

	return nil
}

// getLatest gets the latest root after checking it is consistent with the previous root.
func (a *auditor) getLatest(ctx context.Context) (*types.LogRootV1, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	r, err := a.log.LatestRoot(ctx, &log.LatestRootRequest{LastTreeSize: int64(a.trustedRoot.TreeSize)})
	if err != nil {
		return nil, err
	}

	proof := [][]byte{{}}
	if a.trustedRoot.TreeSize > 0 {
		proof = r.GetProof().GetHashes()
	}
	newRoot, err := a.v.VerifyRoot(&a.trustedRoot, r.GetRoot(), proof)
	if err != nil {
		return nil, fmt.Errorf("failed to verify log root consistency: %v", err)
	}
	return newRoot, nil
}

// getIndex gets the data at the given index and checks the proof it is committed to
// by the given root.
func (a *auditor) getIndex(ctx context.Context, root *types.LogRootV1, index int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	r, err := a.log.GetEntry(ctx, &log.GetEntryRequest{TreeSize: int64(root.TreeSize), Index: index})
	if err != nil {
		return nil, err
	}

	data := r.GetData()
	if err := a.v.VerifyInclusionAtIndex(root, data, index, r.GetProof().GetHashes()); err != nil {
		return nil, fmt.Errorf("failed to verify leaf inclusion: %v", err)
	}
	return data, nil
}

func (a *auditor) close() error {
	return a.con.Close()
}

func main() {
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *connectTimeout)
	defer cancel()

	a := new(ctx)
	defer a.close()

	glog.Infof("auditor running, poll interval: %v", *pollInterval)
	ticker := time.NewTicker(*pollInterval)
	for {
		select {
		case <-ticker.C:
			glog.V(2).Info("Tick")
			if err := a.checkLatest(context.Background()); err != nil {
				glog.Warningf("error checking latest root: %v", err)
			}
		case <-context.Background().Done():
			glog.Info("context done - finishing")
			ticker.Stop()
			return
		}
	}
}
