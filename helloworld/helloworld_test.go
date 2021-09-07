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

package helloworld

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/google/trillian-examples/formats/log"
	p "github.com/google/trillian-examples/helloworld/personality"
	"golang.org/x/mod/sumdb/note"
)

const (
	// testPrivateKey is the personalities key for signing its checkpoints.
	testPrivateKey = "PRIVATE+KEY+helloworld+b51acf1b+ASW28PXJDCV8klh7JeacIgfJR3/Q60dklasmgnv4c9I7"
	// testPublicKey is used for verifying the signatures on the checkpoints from
	// the personality.
	testPublicKey = "helloworld+b51acf1b+AZ2ZM0ZQ69GwDUyO7/x0JyLo09y3geyufyN1mFFMeUH3"
)

var (
	seed         = flag.String("seed", time.Now().Format(time.UnixDate), "Seed for leaf randomness")
	trillianAddr = flag.String("trillian", "localhost:50054", "Host:port of Trillian Log RPC server")
	treeID       = flag.Int64("tree_id", 0, "Tree ID")
)

func mustGetSigner(t *testing.T) note.Signer {
	t.Helper()
	s, err := note.NewSigner(testPrivateKey)
	if err != nil {
		t.Fatalf("Failed to create signer: %q", err)
	}
	return s
}

func mustGetVerifier(t *testing.T) note.Verifier {
	t.Helper()
	v, err := note.NewVerifier(testPublicKey)
	if err != nil {
		t.Fatalf("Failed to create verifier: %q", err)
	}
	return v
}

func mustOpenCheckpoint(t *testing.T, cRaw []byte) *log.Checkpoint {
	t.Helper()
	cn, err := log.ParseCheckpoint(cRaw, "Hello World Log", mustGetVerifier(t))
	if err != nil {
		t.Fatalf("Failed to open checkpoint: %q", err)
	}
	return cn.Checkpoint
}

// TestAppend appends a random entry to the log and ensures that the
// checkpoint updates properly (locally on the personality's side).
func TestAppend(t *testing.T) {
	if *treeID == 0 {
		t.Skip("--tree_id flag unset, skipping test")
	}

	name := "testAppend"
	t.Run(name, func(t *testing.T) {
		ctx := context.Background()
		personality, err := p.NewPersonality(*trillianAddr, *treeID, mustGetSigner(t))
		if err != nil {
			t.Fatalf(err.Error())
		}
		chkptOldRaw, err := personality.GetChkpt(ctx)
		if err != nil {
			t.Fatalf(err.Error())
		}
		chkptOld := mustOpenCheckpoint(t, chkptOldRaw)
		// Add a random entry so we can be sure it's new.
		entry := make([]byte, 10)
		rand.Seed(time.Now().UnixNano())
		rand.Read(entry)
		chkptNewRaw, err := personality.Append(ctx, entry)
		if err != nil {
			t.Fatalf(err.Error())
		}
		chkptNew := mustOpenCheckpoint(t, chkptNewRaw)
		if chkptNew.Size <= chkptOld.Size {
			t.Errorf("the log didn't grow properly in %v", name)
		}
		fmt.Printf("success in %v, new log size is %v\n", name, chkptNew.Size)
	})
}

// TestUpdate appends a random entry to the log and ensures that the
// checkpoint updates properly for both the personality and the verifier.
func TestUpdate(t *testing.T) {
	if *treeID == 0 {
		t.Skip("--tree_id flag unset, skipping test")
	}

	name := "testUpdate"
	t.Run(name, func(t *testing.T) {
		ctx := context.Background()
		personality, err := p.NewPersonality(*trillianAddr, *treeID, mustGetSigner(t))
		if err != nil {
			t.Fatalf(err.Error())
		}
		client := NewClient(personality, mustGetVerifier(t))
		chkptRaw, err := personality.GetChkpt(ctx)
		if err != nil {
			t.Fatalf(err.Error())
		}
		client.chkpt = mustOpenCheckpoint(t, chkptRaw)
		entry := make([]byte, 10)
		rand.Seed(time.Now().UnixNano())
		rand.Read(entry)
		personality.Append(ctx, entry)
		chkptNewRaw, pf, err := personality.UpdateChkpt(ctx, client.chkpt.Size)
		if err != nil {
			t.Fatalf(err.Error())
		}
		got := client.UpdateChkpt(chkptNewRaw, pf)
		if got != nil {
			t.Errorf("verifier failed to update checkpoint: %q", err)
		}
		chkptNew := mustOpenCheckpoint(t, chkptNewRaw)
		fmt.Printf("success in %v, new log size is %v\n", name, chkptNew.Size)
	})
}

// TestIncl tests inclusion proof checking for entries that both are and
// aren't in the log.
func TestIncl(t *testing.T) {
	if *treeID == 0 {
		t.Skip("--tree_id flag unset, skipping test")
	}

	tests := []struct {
		name         string
		addEntries   []string
		checkEntries []string
		wants        []bool
	}{
		{
			name:         "all there",
			addEntries:   []string{"a", "b", "c", "d"},
			checkEntries: []string{"a", "b", "c", "d"},
			wants:        []bool{true, true, true, true},
		},
		{
			name:         "all missing",
			addEntries:   []string{"e", "f", "g", "h"},
			checkEntries: []string{"w", "x", "y", "z"},
			wants:        []bool{false, false, false, false},
		},
		{
			name:         "mixed bag",
			addEntries:   []string{"i", "j", "k", "l"},
			checkEntries: []string{"i", "j", "y", "z"},
			wants:        []bool{true, true, false, false},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			personality, err := p.NewPersonality(*trillianAddr, *treeID, mustGetSigner(t))
			if err != nil {
				t.Fatalf(err.Error())
			}
			var chkptRaw []byte
			// Append all the entries we plan to add, folding in
			// the seed to avoid duplication of entries across tests.
			for _, entry := range test.addEntries {
				entry = entry + *seed
				bs := []byte(entry)
				chkptRaw, err = personality.Append(ctx, bs)
				if err != nil {
					// If the checkpoint didn't update that's a problem.
					t.Fatalf(err.Error())
				}
			}
			client := NewClient(personality, mustGetVerifier(t))
			// For the purposes of the test let's skip having the
			// verifier update the right way and just assign their checkpoint.
			client.chkpt = mustOpenCheckpoint(t, chkptRaw)
			// Then prove and check inclusion of the other entries.
			for i := range test.checkEntries {
				entry := test.checkEntries[i] + *seed
				bs := []byte(entry)
				// Ignore error here since it's okay if we
				// don't have a valid inclusion proof (testing
				// on entries that aren't there).
				pf, err := personality.ProveIncl(ctx, client.chkpt.Size, bs)
				got := false
				if err == nil {
					got = client.VerIncl(bs, pf)
				}
				if got != test.wants[i] {
					t.Errorf("%v: got %v, want %v", test.name, got, test.wants[i])
				}
			}
			fmt.Printf("testIncl: all good for the %v test!\n", test.name)
		})
	}
}
