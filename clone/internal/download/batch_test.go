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
	"fmt"
	"strconv"
	"testing"
	"time"
)

func TestFetchWorkerRun(t *testing.T) {
	for _, test := range []struct {
		name            string
		first, treeSize uint64
		batchSize       uint
	}{
		{
			name:      "smallest batch",
			first:     0,
			treeSize:  10,
			batchSize: 1,
		},
		{
			name:      "larger batch",
			first:     0,
			treeSize:  110,
			batchSize: 10,
		},
		{
			name:      "batch size non-divisor of range",
			first:     0,
			treeSize:  107,
			batchSize: 10,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			wrc := make(chan workerResult)

			fakeFetch := func(start uint64, leaves [][]byte) error {
				return nil
			}
			fw := fetchWorker{
				label:      test.name,
				start:      test.first,
				treeSize:   test.treeSize,
				increment:  uint64(test.batchSize),
				count:      test.batchSize,
				out:        wrc,
				batchFetch: fakeFetch,
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
	for _, test := range []struct {
		name            string
		first, treeSize uint64
		batchSize       uint
		workers         uint
	}{
		{
			name:      "smallest batch",
			first:     0,
			treeSize:  10,
			batchSize: 1,
			workers:   1,
		},
		{
			name:      "larger batch",
			first:     0,
			treeSize:  110,
			batchSize: 10,
			workers:   4,
		},
		{
			name:      "batch size non-divisor of range",
			first:     0,
			treeSize:  107,
			batchSize: 10,
			workers:   4,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			brc := make(chan BulkResult)
			fakeFetch := func(start uint64, leaves [][]byte) error {
				for i := range leaves {
					leaves[i] = []byte(strconv.Itoa(int(start) + i))
				}
				return nil
			}
			go Bulk(context.Background(), test.first, test.treeSize, fakeFetch, test.workers, test.batchSize, brc)

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

	fakeFetch := func(start uint64, leaves [][]byte) error {
		return nil
	}
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

func BenchmarkBulk(b *testing.B) {
	for _, test := range []struct {
		workers    uint
		batchSize  uint
		fetchDelay time.Duration
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
	} {
		b.Run(fmt.Sprintf("w=%d,b=%d,delay=%s", test.workers, test.batchSize, test.fetchDelay), func(b *testing.B) {
			brc := make(chan BulkResult, 10)
			var first uint64

			fakeFetch := func(start uint64, leaves [][]byte) error {
				time.Sleep(test.fetchDelay)
				for i := range leaves {
					// Allocate a non-trivial amount of memory for the leaf.
					leaf := make([]byte, 1024)
					leaves[i] = leaf
				}
				return nil
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

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
