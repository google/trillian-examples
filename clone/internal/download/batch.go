// Copyright 2021 Google LLC
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

// download contains a library for downloading data from logs.
package download

import (
	"context"
	"fmt"
	"time"

	backoff "github.com/cenkalti/backoff/v4"
	"github.com/golang/glog"
)

// BatchFetch should be implemented to provide a mechanism to fetch a range of leaves.
// Enough leaves should be fetched to fully fill `leaves`, or an error should be returned.
type BatchFetch func(start uint64, leaves [][]byte) error

// BulkResult combines a downloaded leaf, or the error found when trying to obtain the leaf.
type BulkResult struct {
	Leaf []byte
	Err  error
}

// Bulk keeps downloading leaves starting from `first`, using the given leaf fetcher.
// The number of workers and the batch size to use for each of the fetch requests are also specified.
// The resulting leaves are returned in order over `leafChan`, and any terminal errors are returned via `errc`.
// Internally this uses exponential backoff on the workers to download as fast as possible, but no faster.
// Bulk takes ownership of `rc` and will close it when no more values will be written.
func Bulk(ctx context.Context, first, last uint64, batchFetch BatchFetch, workers, batchSize uint, rc chan<- BulkResult) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer close(rc)
	// Each worker gets its own unbuffered channel to make sure it can only be at most one ahead.
	// This prevents lots of wasted work happening if one shard gets stuck.
	rangeChans := make([]chan workerResult, workers)

	increment := workers * batchSize
	for i := uint(0); i < workers; i++ {
		rangeChans[i] = make(chan workerResult)
		start := first + uint64(i*batchSize)
		go fetchWorker{
			label:      fmt.Sprintf("worker %d", i),
			start:      start,
			last:       last,
			count:      batchSize,
			increment:  uint64(increment),
			out:        rangeChans[i],
			batchFetch: batchFetch,
		}.run(ctx)
	}

	lastStart := last - uint64(batchSize)
	var r workerResult
	// Perpetually round-robin through the sharded ranges.
	for i := 0; ; i = (i + 1) % int(workers) {
		select {
		case <-ctx.Done():
			rc <- BulkResult{
				Leaf: nil,
				Err:  ctx.Err(),
			}
			return
		case r = <-rangeChans[i]:
		}
		if r.err != nil {
			rc <- BulkResult{
				Leaf: nil,
				Err:  r.err,
			}
			return
		}
		for _, l := range r.leaves {
			rc <- BulkResult{
				Leaf: l,
				Err:  nil,
			}
		}
		if r.start >= lastStart {
			return
		}
	}
}

type workerResult struct {
	start  uint64
	leaves [][]byte
	err    error
}

type fetchWorker struct {
	label                  string
	start, last, increment uint64
	count                  uint
	out                    chan<- workerResult
	batchFetch             BatchFetch
}

func (w fetchWorker) run(ctx context.Context) {
	defer close(w.out)
	// TODO(mhutchinson): Consider some way to reset this after intermittent connectivity issue.
	// If this is pushed in the loop then it fixes this issue, but at the cost that the worker
	// will never reach a stable rate if it is asked to back off. This is optimized for being
	// gentle to the logs, which is a reasonable default for a happy ecosystem.
	bo := backoff.NewExponentialBackOff()
	for {
		if w.start > w.last {
			return
		}
		count := w.count
		if left := w.last - w.start; left < uint64(count) {
			count = uint(left)
		}

		leaves := make([][]byte, count)
		var c workerResult
		operation := func() error {
			err := w.batchFetch(w.start, leaves)
			if err != nil {
				return fmt.Errorf("LeafFetcher.Batch(%d, %d): %w", w.start, w.count, err)
			}
			c = workerResult{
				start:  w.start,
				leaves: leaves,
				err:    nil,
			}
			return nil
		}
		c.err = backoff.RetryNotify(operation, bo, func(e error, _ time.Duration) {
			glog.V(1).Infof("%s: Retryable error getting data: %q", w.label, e)
		})
		select {
		case <-ctx.Done():
			return
		case w.out <- c:
		}
		w.start += w.increment
	}
}
