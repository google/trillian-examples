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

// Bulk keeps downloading leaves starting from `first`, using the given leaf fetcher.
// The number of workers and the batch size to use for each of the fetch requests are also specified.
// The resulting leaves are returned in order over `leafChan`, and any terminal errors are returned via `errc`.
// Internally this uses exponential backoff on the workers to download as fast as possible, but no faster.
func Bulk(ctx context.Context, first uint64, batchFetch BatchFetch, workers, batchSize uint, leafChan chan<- []byte, errc chan<- error) {
	// Each worker gets its own unbuffered channel to make sure it can only be at most one ahead.
	// This prevents lots of wasted work happening if one shard gets stuck.
	rangeChans := make([]chan leafRange, workers)

	increment := workers * batchSize
	for i := uint(0); i < workers; i++ {
		rangeChans[i] = make(chan leafRange)
		start := first + uint64(i*batchSize)
		go fetchWorker{
			label:      fmt.Sprintf("worker %d", i),
			start:      start,
			count:      batchSize,
			increment:  uint64(increment),
			out:        rangeChans[i],
			errc:       errc,
			batchFetch: batchFetch,
		}.run(ctx)
	}

	var r leafRange
	// Perpetually round-robin through the sharded ranges.
	for i := 0; ; i = (i + 1) % int(workers) {
		select {
		case <-ctx.Done():
			errc <- ctx.Err()
			return
		case r = <-rangeChans[i]:
		}
		for _, l := range r.leaves {
			leafChan <- l
		}
	}
}

type leafRange struct {
	start  uint64
	leaves [][]byte
}

type fetchWorker struct {
	label            string
	start, increment uint64
	count            uint
	out              chan<- leafRange
	errc             chan<- error
	batchFetch       BatchFetch
}

func (w fetchWorker) run(ctx context.Context) {
	// TODO(mhutchinson): Consider some way to reset this after intermittent connectivity issue.
	// If this is pushed in the loop then it fixes this issue, but at the cost that the worker
	// will never reach a stable rate if it is asked to back off. This is optimized for being
	// gentle to the logs, which is a reasonable default for a happy ecosystem.
	bo := backoff.NewExponentialBackOff()
	for {
		leaves := make([][]byte, w.count)
		var c leafRange
		operation := func() error {
			err := w.batchFetch(w.start, leaves)
			if err != nil {
				return fmt.Errorf("LeafFetcher.Batch(%d, %d): %w", w.start, w.count, err)
			}
			c = leafRange{
				start:  w.start,
				leaves: leaves,
			}
			return nil
		}
		err := backoff.RetryNotify(operation, bo, func(e error, _ time.Duration) {
			glog.V(1).Infof("%s: Retryable error getting data: %q", w.label, e)
		})
		if err != nil {
			w.errc <- err
		} else {
			select {
			case <-ctx.Done():
				return
			case w.out <- c:
			}
		}
		w.start += w.increment
	}
}
