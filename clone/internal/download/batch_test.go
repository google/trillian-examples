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

package download

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func getFakeFetch(fetchOnly uint64) func(start uint64, leaves [][]byte) (uint64, error) {
	return func(start uint64, leaves [][]byte) (uint64, error) {
		for i := range leaves {
			if uint64(i) == fetchOnly {
				return uint64(i), nil
			}
			leaves[i] = []byte(strconv.Itoa(int(start) + i))
		}
		return uint64(len(leaves)), nil
	}
}

type testCase struct {
	name            string
	first, treeSize uint64
	batchSize       uint
	workers         uint
	wantErr         bool
	fakeFetch       func(start uint64, leaves [][]byte) (uint64, error)
}

func TestFetchWorkerRun(t *testing.T) {
	for _, test := range []testCase{
		{
			name:      "smallest batch",
			first:     0,
			treeSize:  10,
			batchSize: 1,
			fakeFetch: getFakeFetch(1),
		},
		{
			name:      "larger batch",
			first:     0,
			treeSize:  110,
			batchSize: 10,
			fakeFetch: getFakeFetch(10),
		},
		{
			name:      "bigger batch than tree",
			first:     0,
			treeSize:  9,
			batchSize: 10,
			fakeFetch: getFakeFetch(10),
		},
		{
			name:      "batch size non-divisor of range",
			first:     0,
			treeSize:  107,
			batchSize: 10,
			fakeFetch: getFakeFetch(10),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			wrc := make(chan workerResult)

			fw := fetchWorker{
				label:      test.name,
				start:      test.first,
				treeSize:   test.treeSize,
				increment:  uint64(test.batchSize),
				count:      test.batchSize,
				out:        wrc,
				batchFetch: test.fakeFetch,
			}

			go fw.run(context.Background())

			var seen, i int
			for r := range wrc {
				if r.err != nil {
					t.Fatal(r.err)
				}
				if got, want := r.start, uint64(i*int(test.batchSize)); got != want {
					t.Errorf("%d got != want (%d != %d)", i, got, want)
				}
				seen = seen + len(r.leaves)
				i++
			}
			if seen != int(test.treeSize) {
				t.Errorf("expected to see %d leaves but saw %d", test.treeSize, seen)
			}
		})
	}
}

func TestBulk(t *testing.T) {
	for _, test := range []testCase{
		{
			name:      "smallest batch",
			first:     0,
			treeSize:  10,
			batchSize: 1,
			workers:   1,
			fakeFetch: getFakeFetch(1),
		},
		{
			name:      "larger batch",
			first:     0,
			treeSize:  110,
			batchSize: 10,
			workers:   4,
			fakeFetch: getFakeFetch(10),
		},
		{
			name:      "bigger batch than tree",
			first:     0,
			treeSize:  9,
			batchSize: 10,
			workers:   1,
			fakeFetch: getFakeFetch(10),
		},
		{
			name:      "batch size equals tree size",
			first:     0,
			treeSize:  10,
			batchSize: 10,
			workers:   1,
			fakeFetch: getFakeFetch(10),
		},
		{
			name:      "batch size non-divisor of range",
			first:     0,
			treeSize:  107,
			batchSize: 10,
			workers:   4,
			fakeFetch: getFakeFetch(10),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			brc := make(chan BulkResult)

			go Bulk(context.Background(), test.first, test.treeSize, test.fakeFetch, test.workers, test.batchSize, brc)

			i := 0
			for br := range brc {
				if br.Err != nil {
					t.Fatal(br.Err)
				}
				if got, want := string(br.Leaf), strconv.Itoa(i); got != want {
					t.Errorf("%d got != want (%q != %q)", i, got, want)
				}
				i++
			}
			if i != int(test.treeSize) {
				t.Errorf("expected %d leaves, got %d", test.treeSize, i)
			}
		})
	}
}

func TestBulkCancelled(t *testing.T) {
	brc := make(chan BulkResult, 10)
	var first uint64
	var treeSize uint64 = 1000
	var workers uint = 4
	var batchSize uint = 10

	fakeFetch := getFakeFetch(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go Bulk(ctx, first, treeSize, fakeFetch, workers, batchSize, brc)

	seen := 0
	for i := 0; i < int(treeSize); i++ {
		br := <-brc
		if br.Err != nil {
			break
		}
		seen++
		if seen == 10 {
			cancel()
		}
	}
	if seen == int(treeSize) {
		t.Error("Expected cancellation to prevent all leaves being read")
	}
}

func TestBulkIncomplete(t *testing.T) {
	for _, test := range []testCase{
		{
			name:      "incomplete first batch",
			first:     0,
			treeSize:  100,
			batchSize: 10,
			workers:   4,
			wantErr:   false,
			fakeFetch: func(start uint64, leaves [][]byte) (uint64, error) {
				fetched := uint64(len(leaves))
				for i := range leaves {
					leaves[i] = []byte(strconv.Itoa(int(start) + i))
					if start == 0 && i == 4 {
						fetched = 5
						break
					}
				}
				return fetched, nil
			},
		},
		{
			name:      "incomplete last batch",
			first:     0,
			treeSize:  100,
			batchSize: 10,
			workers:   4,
			wantErr:   true,
			fakeFetch: func(start uint64, leaves [][]byte) (uint64, error) {
				fetched := uint64(len(leaves))
				for i := range leaves {
					leaves[i] = []byte(strconv.Itoa(int(start) + i))
					if start == 90 && i == 4 {
						fetched = 5
						break
					}
				}
				return fetched, nil
			},
		},
		{
			name:      "incomplete middle batch",
			first:     0,
			treeSize:  100,
			batchSize: 10,
			workers:   4,
			wantErr:   true,
			fakeFetch: func(start uint64, leaves [][]byte) (uint64, error) {
				fetched := uint64(len(leaves))
				for i := range leaves {
					leaves[i] = []byte(strconv.Itoa(int(start) + i))
					if start == 50 && i == 4 {
						fetched = 5
						break
					}
				}
				return fetched, nil
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			brc := make(chan BulkResult)
			go Bulk(context.Background(), test.first, test.treeSize, test.fakeFetch, test.workers, test.batchSize, brc)

			i := 0
			var err error
			for br := range brc {
				if br.Err != nil && !test.wantErr {
					t.Fatal(br.Err)
				}
				if br.Err != nil && test.wantErr {
					err = br.Err
				}
				if got, want := string(br.Leaf), strconv.Itoa(i); got != want && err == nil {
					t.Fatalf("%d got != want (%q != %q)", i, got, want)
				}
				i++
			}
			if err == nil && test.wantErr {
				t.Errorf("expected error, got none")
			}
			if err != nil && !test.wantErr {
				t.Errorf("unexpected error: %v", err)
			}
			if i != int(test.treeSize) && !test.wantErr {
				t.Errorf("expected %d leaves, got %d", test.treeSize, i)
			}
		})
	}
}

func BenchmarkBulk(b *testing.B) {
	for _, test := range []struct {
		workers    uint
		batchSize  uint
		fetchDelay time.Duration
		quota      int64
	}{
		{
			workers:    20,
			batchSize:  10,
			fetchDelay: 50 * time.Microsecond,
		},
		{
			workers:    20,
			batchSize:  1,
			fetchDelay: 50 * time.Microsecond,
		},
		{
			workers:    1,
			batchSize:  1,
			fetchDelay: 50 * time.Microsecond,
		},
		{
			workers:    1,
			batchSize:  200,
			fetchDelay: 50 * time.Microsecond,
		},
		{
			workers:    20,
			batchSize:  10,
			fetchDelay: 50 * time.Microsecond,
			quota:      1000,
		},
	} {
		b.Run(fmt.Sprintf("w=%d,b=%d,delay=%s,q=%d", test.workers, test.batchSize, test.fetchDelay, test.quota), func(b *testing.B) {
			brc := make(chan BulkResult, 10)
			var first uint64

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			take := func(n int) error { return nil }
			if test.quota > 0 {
				th := throttle{
					quota:  test.quota,
					refill: test.quota,
				}
				go th.startRefillLoop(ctx)
				take = th.take
			}

			fakeFetch := func(start uint64, leaves [][]byte) (uint64, error) {
				time.Sleep(test.fetchDelay)
				if err := take(len(leaves)); err != nil {
					return 0, err
				}
				for i := range leaves {
					// Allocate a non-trivial amount of memory for the leaf.
					leaf := make([]byte, 1024)
					leaves[i] = leaf
				}
				return uint64(len(leaves)), nil
			}

			const consumeSize = 1000
			go Bulk(ctx, first, uint64(b.N*consumeSize), fakeFetch, test.workers, test.batchSize, brc)

			for n := 0; n < b.N; n++ {
				for i := 0; i < consumeSize; i++ {
					br := <-brc
					if br.Err != nil {
						b.Fatal(br.Err)
					}
				}
			}
		})
	}
}

type throttle struct {
	quota  int64
	refill int64
}

func (t *throttle) take(n int) error {
	if atomic.AddInt64(&t.quota, int64(n*-1)) > 0 {
		return nil
	}
	return errors.New("out of quota")
}

func (t *throttle) startRefillLoop(ctx context.Context) {
	tik := time.NewTicker(10 * time.Millisecond)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tik.C:
			atomic.StoreInt64(&t.quota, t.refill)
		}
	}
}
