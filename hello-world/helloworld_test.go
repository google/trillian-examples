// Copyright 2020 Google LLC. All Rights Reserved.
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
    "math/rand"
    "flag"
    "fmt"
    "testing"
    "time"

    p "github.com/google/trillian-examples/hello-world/personality"
)

var (
    trillianAddr = flag.String("trillian", "localhost:50054", "Host:port of Trillian Log RPC server")
    treeID = flag.Int64("tree_id", 0, "Tree ID")
)

// TestAppend appends a random entry to the log and ensures that the
// checkpoint updates properly (locally on the personality's side).
func TestAppend(t *testing.T) {
    if len(*trillianAddr) == 0 || *treeID == 0 {
	t.Skip("--trillian flag unset, skipping test")
    }

    name := "testAppend"
    t.Run(name, func(t *testing.T) {
	ctx := context.Background()
	personality := p.NewPersonality(*trillianAddr, *treeID)
	chkptOld := personality.GetChkpt(ctx)
	// Add a random entry so we can be sure it's new.
	entry := make([]byte, 10)
	rand.Seed(time.Now().UnixNano())
	rand.Read(entry)
	chkptNew := personality.Append(ctx, entry)
	if chkptNew.LogSize <= chkptOld.LogSize {
	    t.Errorf("the log didn't grow properly in %v", name)
	}
	fmt.Printf("success in %v, new log size is %v\n", name, chkptNew.LogSize)
    })
}

// TestUpdate appends a random entry to the log and ensures that the
// checkpoint updates properly for both the personality and the verifier.
func TestUpdate(t *testing.T) {
    if len(*trillianAddr) == 0 || *treeID == 0 {
	t.Skip("--trillian flag unset, skipping test")
    }

    name := "testUpdate"
    t.Run(name, func(t *testing.T) {
	ctx := context.Background()
	personality := p.NewPersonality(*trillianAddr, *treeID)
	client := NewClient(personality)
	chkpt := personality.GetChkpt(ctx)
	client.chkpt = chkpt
	entry := make([]byte, 10)
	rand.Seed(time.Now().UnixNano())
	rand.Read(entry)
	personality.Append(ctx, entry)
	chkptNew, pf := personality.UpdateChkpt(ctx, chkpt)
	got := client.UpdateChkpt(chkptNew, pf)
	if !got {
	    t.Errorf("verifier failed to update checkpoint")
	}
	fmt.Printf("success in %v, new log size is %v\n", name, chkpt.LogSize)
    })
}

// TestIncl tests inclusion proof checking for entries that both are and
// aren't in the log.
func TestIncl(t *testing.T) {
    if len(*trillianAddr) == 0 || *treeID == 0{
	t.Skip("--trillian flag unset, skipping test")
    }

    tests := []struct {
	name           string
	addEntries     []string
	checkEntries   []string
	wants          []bool
    }{
	{
	    name:         "all there",
	    addEntries:   []string{"a", "b", "c", "d"},
	    checkEntries: []string{"a", "b", "c", "d"},
	    wants:        []bool{true, true, true, true},
	},
	{
	    name:         "all missing",
	    addEntries:   []string{"a", "b", "c", "d"},
	    checkEntries: []string{"w", "x", "y", "z"},
	    wants:        []bool{false, false, false, false},
	},
	{
	    name:         "mixed bag",
	    addEntries:   []string{"a", "b", "c", "d"},
	    checkEntries: []string{"a", "b", "y", "z"},
	    wants:        []bool{true, true, false, false},
	},
    }

    for _, test := range tests {
	test := test
	t.Run(test.name, func(t *testing.T) {
	    ctx := context.Background()
	    personality := p.NewPersonality(*trillianAddr, *treeID)
	    var chkpt p.Chkpt
            // Append all the entries we plan to add (some of them might be
            // there already).
	    for _, entry := range test.addEntries {
		bs := []byte(entry)
		chkpt = personality.Append(ctx, bs)
	    }
	    client := NewClient(personality)
	    // For the purposes of the test let's skip having the verifier 
	    // update the right way and just assign their checkpoint.
	    client.chkpt = chkpt
	    // Then prove and check inclusion of the other entries.
	    for i, _ := range test.checkEntries {
		entry := test.checkEntries[i]
		bs := []byte(entry)
		pf := personality.ProveIncl(ctx, chkpt, bs)
		got := client.VerIncl(bs, pf)
		if got != test.wants[i] {
		    t.Errorf("%v: got %v, want %v", test.name, got, test.wants[i])
		}
	    }
	    fmt.Printf("testIncl: all good for the %v test!\n", test.name)
	})
    }
}
