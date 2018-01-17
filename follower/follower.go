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

package follower

import (
	"bytes"
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/golang/glog"
	"github.com/google/trillian"
)

type FollowerOpts struct {
	BatchSize uint64
}

type Follower struct {
	logID int64
	gc    *ethclient.Client
	tc    trillian.TrillianLogClient

	opts FollowerOpts
}

func New(gc *ethclient.Client, tc trillian.TrillianLogClient, logID int64, opts FollowerOpts) *Follower {
	if opts.BatchSize <= 0 {
		opts.BatchSize = 100
	}
	return &Follower{
		logID: logID,
		gc:    gc,
		tc:    tc,
		opts:  opts,
	}
}

func (f *Follower) Follow(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	nextBlock := int64(-1)
nextAttempt:
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		// Get initial STH, if necessary:
		if nextBlock < 0 {
			sth, err := f.tc.GetLatestSignedLogRoot(ctx, &trillian.GetLatestSignedLogRootRequest{LogId: f.logID})
			if err != nil {
				continue
			}
			nextBlock = sth.SignedLogRoot.TreeSize
			glog.Infof("Got starting STH of:\n%+v", sth)
		}

		sync, err := f.gc.SyncProgress(ctx)
		if err != nil {
			glog.Errorf("Failed to get sync progress: %v", err)
			continue
		}
		if sync == nil {
			glog.Errorf("No sync progress, perhaps geth hasn't got any data yet?")
			continue
		}

		if sync.CurrentBlock <= uint64(nextBlock) {
			continue
		}
		for ; uint64(nextBlock) < sync.CurrentBlock; nextBlock++ {
			b, err := f.gc.BlockByNumber(ctx, big.NewInt(nextBlock))
			if err != nil {
				glog.Errorf("Failed to get block %v: %v", nextBlock, err)
				continue nextAttempt
			}
			raw := bytes.Buffer{}
			if err := b.EncodeRLP(&raw); err != nil {
				glog.Errorf("Error serialising block %v: %v", nextBlock, err)
				continue nextAttempt
			}
			leaf := &trillian.LogLeaf{
				LeafValue: raw.Bytes(),
			}
			// TODO(al): actually batch.
			// XXX obviously, this is going to result in the blocks being all
			// out-of-order with respect to the chain due to Trillian sequencing.
			// either we can use the Mirroring APIs once they're ready, or use the
			// hash chain hashes to sort it out in the wash when we construct the
			// Map from the entries in the Log.
			if _, err := f.tc.QueueLeaves(ctx, &trillian.QueueLeavesRequest{LogId: f.logID, Leaves: []*trillian.LogLeaf{leaf}}); err != nil {
				glog.Errorf("Failed to Queue block %v: %v", nextBlock, err)
				continue nextAttempt
			}
			if nextBlock%1000 == 0 {
				glog.Infof("Copied to %v", nextBlock)
			}
		}
	}
}
