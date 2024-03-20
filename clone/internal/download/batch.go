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

// Package download contains a library for downloading data from logs.
package download

import (
	"context"
	"errors"
	"fmt"
	"time"

	backoff "github.com/cenkalti/backoff/v4"
	"github.com/golang/glog"
)

// BatchFetch should be implemented to provide a mechanism to fetch a range of leaves.
// It should return the number of leaves fetched, or an error if the fetch failed.
type BatchFetch func(start uint64, leaves [][]byte) (uint64, error)

// BulkResult combines a downloaded leaf, or the error found when trying to obtain the leaf.
type BulkResult struct {
	Leaf []byte
	Err  error
}

// Bulk keeps downloading leaves starting from `first`, using the given leaf fetcher.
// The number of workers and the batch size to use for each of the fetch requests are also specified.
// The resulting leaves or terminal errors are returned in order over `rc`. Bulk takes ownership of `rc` and will close it when no more values will be written.
// Internally this uses exponential backoff on the workers to download as fast as possible, but no faster.
func Bulk(ctx context.Context, first, treeSize uint64, batchFetch BatchFetch, workers, batchSize uint, rc chan<- BulkResult) {
	defer close(rc)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Each worker gets its own unbuffered channel to make sure it can only be at most one ahead.
	// This prevents lots of wasted work happening if one shard gets stuck.
	rangeChans := make([]chan workerResult, workers)
	increment := workers * batchSize

	badAlignErr := errors.New("not aligned")
	align := func() (uint64, error) {
		if left := treeSize - first; left < uint64(batchSize) {
			batchSize = uint(left)
		}
		glog.Infof("Attempting to align by making request [%d, %d)", first, first+uint64(batchSize))
		leaves := make([][]byte, batchSize)
		fetched, err := batchFetch(first, leaves)
		if err != nil {
			glog.Warningf("Failed to fetch batch: %v", err)
			return 0, err
		}
		for i := 0; i < int(fetched); i++ {
			rc <- BulkResult{
				Leaf: leaves[i],
				Err:  nil,
			}
		}
		first += fetched
		if fetched != uint64(batchSize) {
			glog.Warningf("Received partial batch (expected %d, got %d)", batchSize, fetched)
			return first, badAlignErr
		}
		glog.Infof("Received full batch (expected %d, got %d)", batchSize, fetched)
		return first, nil
	}

	var err error
	if first, err = align(); err != nil {
		if err != badAlignErr {
			glog.Errorf("Failed to align: %v", err)
			return
		}
		// We failed to align once, but let's try a second time. If this works then
		// we're aligned with how the log wants to server chunks of entries. If it
		// fails then we can guess that the desired batch size is not configured well.
		if first, err = align(); err != nil {
			glog.Errorf("Failed to align after two attempts, consider a different batch size? %v", err)
			return
		}
	}

	for i := uint(0); i < workers; i++ {
		rangeChans[i] = make(chan workerResult)
		start := first + uint64(i*batchSize)
		go func(i uint, start uint64) {
			fetchWorker{
				label:      fmt.Sprintf("worker %d", i),
				start:      start,
				treeSize:   treeSize,
				count:      batchSize,
				increment:  uint64(increment),
				out:        rangeChans[i],
				batchFetch: batchFetch,
			}.run(ctx)
		}(i, start)
	}

	var lastStart uint64
	if treeSize > uint64(batchSize) {
		lastStart = treeSize - uint64(batchSize)
	}
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
	start   uint64
	leaves  [][]byte
	fetched uint64
	err     error
}

type fetchWorker struct {
	label                      string
	start, treeSize, increment uint64
	count                      uint
	out                        chan<- workerResult
	batchFetch                 BatchFetch
}

func (w fetchWorker) run(ctx context.Context) {
	glog.V(2).Infof("fetchWorker %q started", w.label)
	defer glog.V(2).Infof("fetchWorker %q finished", w.label)
	defer close(w.out)
	// This exponential backoff is always reset before use in backoff.RetryNotify.
	bo := backoff.NewExponentialBackOff()
	for {
		if w.start >= w.treeSize {
			return
		}
		count := w.count
		if left := w.treeSize - w.start; left < uint64(count) {
			count = uint(left)
		}

		leaves := make([][]byte, count)
		var c workerResult
		operation := func() error {
			fetched, err := w.batchFetch(w.start, leaves)
			if err != nil {
				return fmt.Errorf("LeafFetcher.Batch(%d, %d): %w", w.start, w.count, err)
			}
			c = workerResult{
				start:   w.start,
				leaves:  leaves,
				fetched: fetched,
				err:     nil,
			}
			if fetched != uint64(len(leaves)) {
				return backoff.Permanent(fmt.Errorf("LeafFetcher.Batch(%d, %d): wanted %d leaves but got %d", w.start, w.count, len(leaves), fetched))
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
		if c.err != nil {
			return
		}
		w.start += w.increment
	}
}
