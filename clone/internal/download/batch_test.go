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
	"testing"
)

func TestFetchWorkerRun(t *testing.T) {
	wrc := make(chan workerResult, 10)
	var first uint64
	var batchSize uint = 10

	fakeFetch := func(start uint64, leaves [][]byte) error {
		return nil
	}
	fw := fetchWorker{
		label:      "solipsism",
		start:      first,
		increment:  uint64(batchSize),
		count:      batchSize,
		out:        wrc,
		batchFetch: fakeFetch,
	}

	go fw.run(context.Background())

	for i := 0; i < 10; i++ {
		r := <-wrc
		if r.err != nil {
			t.Fatal(r.err)
		}
		if got, want := r.start, uint64(i*10); got != want {
			t.Errorf("%d got != want (%d != %d)", i, got, want)
		}
	}
}

func TestBulk(t *testing.T) {
	brc := make(chan BulkResult, 10)
	var first uint64
	var workers uint = 4
	var batchSize uint = 10

	fakeFetch := func(start uint64, leaves [][]byte) error {
		for i := range leaves {
			leaves[i] = []byte(fmt.Sprintf("%d.%d", start, i))
		}
		return nil
	}
	go Bulk(context.Background(), first, fakeFetch, workers, batchSize, brc)

	for i := 0; i < 1000; i++ {
		br := <-brc
		if br.Err != nil {
			t.Fatal(br.Err)
		}
		tens := (i / 10) * 10
		units := i % 10
		if got, want := string(br.Leaf), fmt.Sprintf("%d.%d", tens, units); got != want {
			t.Errorf("%d got != want (%q != %q)", i, got, want)
		}
	}
}

func TestBulkCancelled(t *testing.T) {
	brc := make(chan BulkResult, 10)
	var first uint64
	var workers uint = 4
	var batchSize uint = 10

	fakeFetch := func(start uint64, leaves [][]byte) error {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go Bulk(ctx, first, fakeFetch, workers, batchSize, brc)

	seen := 0
	for i := 0; i < 1000; i++ {
		br := <-brc
		if br.Err != nil {
			break
		}
		seen++
		if seen == 10 {
			cancel()
		}
	}
	if seen == 1000 {
		t.Error("Expected cancellation to prevent all leaves being read")
	}
}

func BenchmarkBulk(b *testing.B) {
	brc := make(chan BulkResult, 10)
	var first uint64
	var workers uint = 20
	var batchSize uint = 10

	fakeFetch := func(start uint64, leaves [][]byte) error {
		for i := range leaves {
			// Allocate a non-trivial amount of memory for the leaf.
			leaf := make([]byte, 1024)
			leaves[i] = leaf
		}
		return nil
	}
	go Bulk(context.Background(), first, fakeFetch, workers, batchSize, brc)

	for n := 0; n < b.N; n++ {
		for i := 0; i < 1000; i++ {
			br := <-brc
			if br.Err != nil {
				b.Fatal(br.Err)
			}
		}
	}
}
