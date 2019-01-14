// Copyright 2018 Google Inc. All Rights Reserved.
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

package scanner

import (
	"context"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/gossip/api"
	"github.com/google/trillian-examples/gossip/client"
	"github.com/google/trillian/client/backoff"
)

// FetcherOptions holds configuration options for the Fetcher.
type FetcherOptions struct {
	// Number of entries to request in one batch from the Hub.
	BatchSize int

	// Number of concurrent fetcher workers to run.
	ParallelFetch int

	// [StartIndex, EndIndex) is a log entry range to fetch. If EndIndex == 0,
	// then it gets reassigned to the Hub's current tree size.
	StartIndex int64
	EndIndex   int64

	// Continuous determines whether Fetcher should run indefinitely after
	// reaching EndIndex.  Options below here only apply in Continuous mode.
	Continuous bool

	// QuickInterval is the period within which BatchSize entries have to appear
	// in order to trigger immediate rescanning.  If fewer than BatchSize entries
	// appear in this interval, scanning will back off according to Backoff.
	QuickInterval time.Duration

	// Backoff controls client backoff when continuously fetching from the
	// Hub.
	Backoff *backoff.Backoff
}

// DefaultFetcherOptions returns new FetcherOptions with sensible defaults.
func DefaultFetcherOptions() *FetcherOptions {
	return &FetcherOptions{
		BatchSize:     1000,
		ParallelFetch: 1,
		StartIndex:    0,
		EndIndex:      0,
		Continuous:    false,
	}
}

// Fetcher is a tool that fetches entries from a Gossip Hub.
type Fetcher struct {
	// Client used to talk to the Hub instance.
	client *client.HubClient
	// Configuration options for this Fetcher instance.
	opts *FetcherOptions

	// Current tree head of the Hub this Fetcher sends queries to.
	hth *api.HubTreeHead

	// Stops range generator, which causes the Fetcher to terminate gracefully.
	mu     sync.Mutex
	cancel context.CancelFunc
}

// EntryBatch represents a contiguous range of entries of the Hub.
type EntryBatch struct {
	Start   int64                   // LeafIndex of the first entry in the range.
	Entries []*api.TimestampedEntry // Entries of the range.
}

// fetchRange represents a range of certs to fetch from a Hub.
type fetchRange struct {
	start int64 // inclusive
	end   int64 // inclusive
}

// NewFetcher creates a Fetcher instance using client to talk to the log,
// taking configuration options from opts.
func NewFetcher(client *client.HubClient, opts *FetcherOptions) *Fetcher {
	// Defaults for un-set parameters.
	if opts.Continuous {
		if opts.QuickInterval <= 0 {
			opts.QuickInterval = 45 * time.Second
		}
		if opts.Backoff == nil {
			opts.Backoff = &backoff.Backoff{
				Min:    1 * time.Second,
				Max:    30 * time.Second,
				Factor: 2,
				Jitter: true,
			}
		}
	}

	cancel := func() {} // Protect against calling Stop before Run.
	return &Fetcher{client: client, opts: opts, cancel: cancel}
}

// Prepare caches the Hub's latest tree head if not present and returns it. It also
// adjusts the entry range to fit the size of the tree.
func (f *Fetcher) Prepare(ctx context.Context) (*api.HubTreeHead, error) {
	if f.hth != nil {
		return f.hth, nil
	}

	sth, err := f.client.GetSTH(ctx)
	if err != nil {
		glog.Errorf("GetSTH() failed: %v", err)
		return nil, err
	}
	glog.Infof("Got STH with %d entries", sth.TreeHead.TreeSize)

	size := int64(sth.TreeHead.TreeSize)
	if f.opts.EndIndex == 0 {
		f.opts.EndIndex = size
	} else if f.opts.EndIndex > size {
		glog.Warningf("Reset EndIndex from %d to %d", f.opts.EndIndex, size)
	}
	f.hth = &sth.TreeHead
	return &sth.TreeHead, nil
}

// ForSources is an adapter that invokes the given callback function for each
// entry in a batch that matches one of the given source IDs.
func ForSources(srcIDs [][]byte, cb func(index int64, entry *api.TimestampedEntry)) func(EntryBatch) {
	srcs := make(map[string]bool)
	for _, srcID := range srcIDs {
		srcs[string(srcID)] = true
	}
	return func(batch EntryBatch) {
		for i, entry := range batch.Entries {
			idx := batch.Start + int64(i)
			if entry != nil && srcs[string(entry.SourceID)] {
				cb(idx, entry)
			}
		}
	}
}

// Run performs fetching of the Hub. Blocks until scanning is complete, the
// passed in context is canceled, or Stop is called (and pending work is
// finished). For each successfully fetched batch, runs the fn callback.
func (f *Fetcher) Run(ctx context.Context, fn func(EntryBatch)) error {
	glog.V(1).Info("Starting up Fetcher...")
	if _, err := f.Prepare(ctx); err != nil {
		return err
	}

	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	f.mu.Lock()
	f.cancel = cancel
	f.mu.Unlock()

	// Use a separately-cancelable context for the range generator, so we can
	// close it down (in Stop) but still let the fetchers below run to
	// completion.
	ranges := f.genRanges(cctx)

	// Run fetcher workers.
	var wg sync.WaitGroup
	for w, cnt := 0, f.opts.ParallelFetch; w < cnt; w++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			glog.V(1).Infof("Fetcher worker %d starting...", idx)
			f.runWorker(ctx, ranges, fn)
			glog.V(1).Infof("Fetcher worker %d finished", idx)
		}(w)
	}
	wg.Wait()

	glog.V(1).Info("Fetcher terminated")
	return nil
}

// Stop causes the Fetcher to terminate gracefully. After this call Run will
// try to finish all the started fetches, and then return. Does nothing if
// there was no preceding Run invocation.
func (f *Fetcher) Stop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancel()
}

// genRanges returns a channel of ranges to fetch, and starts a goroutine that
// sends things down this channel. The goroutine terminates when all ranges
// have been generated, or if context is cancelled.
func (f *Fetcher) genRanges(ctx context.Context) <-chan fetchRange {
	batch := int64(f.opts.BatchSize)
	ranges := make(chan fetchRange)

	go func() {
		glog.V(1).Info("Range generator starting")
		defer glog.V(1).Info("Range generator finished")
		defer close(ranges)
		start, end := f.opts.StartIndex, f.opts.EndIndex

		for start < end || f.opts.Continuous {
			// In continuous mode wait for bigger tree head every time we reach the end,
			// including, possibly, the very first iteration.
			if start == end { // Implies f.opts.Continuous == true.
				if err := f.updateTreeHead(ctx); err != nil {
					glog.Warningf("Failed to obtain bigger tree head: %v", err)
					return
				}
				end = f.opts.EndIndex
			}

			batchEnd := start + batch
			if batchEnd > end {
				batchEnd = end
			}
			select {
			case <-ctx.Done():
				glog.Warningf("Cancelling genRanges: %v", ctx.Err())
				return
			case ranges <- fetchRange{start, batchEnd - 1}:
			}
			start = batchEnd
		}
	}()

	return ranges
}

// updateTreeHead waits until a bigger tree head is discovered, and updates the
// Fetcher accordingly.  It will request more entries immediately for a fast-growing
// hub (indicated by more than BatchSize entries appearing in QuickInterval) but
// will perform backoff otherwise.
func (f *Fetcher) updateTreeHead(ctx context.Context) error {
	lastSize := uint64(f.opts.EndIndex)
	targetSize := lastSize + uint64(f.opts.BatchSize)
	quickDeadline := time.Now().Add(f.opts.QuickInterval)

	return f.opts.Backoff.Retry(ctx, func() error {
		sth, err := f.client.GetSTH(ctx)
		if err != nil {
			return err
		}
		glog.V(2).Infof("Got STH with %d entries", sth.TreeHead.TreeSize)

		curSize := sth.TreeHead.TreeSize
		if curSize <= lastSize {
			// Back off due to tree size skew.
			return backoff.RetriableErrorf("tree size has gone backwards: now %d, was %d", curSize, lastSize)
		}
		if time.Now().Before(quickDeadline) {
			if curSize < targetSize {
				// Back off due to slow-growing hub
				return backoff.RetriableErrorf("wait for bigger tree head than %d (last=%d, target=%d)", curSize, lastSize, targetSize)
			}
			// Hub has grown fast enough that we immediate scan for more entries.
			f.opts.Backoff.Reset()
		}
		f.hth = &sth.TreeHead
		f.opts.EndIndex = int64(curSize)
		return nil
	})
}

// runWorker is a worker function for handling fetcher ranges.
// Accepts cert ranges to fetch over the ranges channel, and if the fetch is
// successful sends the corresponding EntryBatch through the fn callback. Will
// retry failed attempts to retrieve ranges until the context is cancelled.
func (f *Fetcher) runWorker(ctx context.Context, ranges <-chan fetchRange, fn func(EntryBatch)) {
	for r := range ranges {
		// Logs MAY return fewer than the number of leaves requested. Only complete
		// if we actually got all the leaves we were expecting.
		for r.start <= r.end {
			// Fetcher.Run() can be cancelled while we are looping over this job.
			if err := ctx.Err(); err != nil {
				glog.Warningf("Worker context closed: %v", err)
				return
			}
			entries, err := f.client.GetEntries(ctx, r.start, r.end)
			if err != nil {
				glog.Errorf("GetEntries(%d,%d) failed: %v", r.start, r.end, err)
				continue
			}
			fn(EntryBatch{Start: r.start, Entries: entries})
			r.start += int64(len(entries))
		}
	}
}
