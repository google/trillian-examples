// Copyright 2023 Google LLC. All Rights Reserved.
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

// verify checks that a cloned SumDB log does not contain any conflicting entries.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/trillian-examples/clone/logdb"
	"github.com/transparency-dev/formats/log"
	"github.com/transparency-dev/merkle/compact"
	"github.com/transparency-dev/merkle/rfc6962"
	"golang.org/x/mod/sumdb/note"
	"k8s.io/klog/v2"

	_ "github.com/go-sql-driver/mysql"
)

var (
	mysqlURI     = flag.String("mysql_uri", "", "URI of the MySQL database containing the log.")
	pollInterval = flag.Duration("poll_interval", 0, "How often to re-verify the contents of the DB. Set to 0 to exit after first verification.")

	// Example leaf:
	// golang.org/x/text v0.3.0 h1:g61tztE5qeGQ89tm6NTjjM9VPIm088od1l6aSorWRWg=
	// golang.org/x/text v0.3.0/go.mod h1:NqM8EUOU14njkJ3fqMW+pc6Ldnwhi/IjpwHt7yyuwOQ=
	//
	line0RE = regexp.MustCompile(`(.*) (.*) h1:(.*)`)
	line1RE = regexp.MustCompile(`(.*) (.*)/go.mod h1:(.*)`)
)

type dataSource interface {
	GetLatestCheckpoint(ctx context.Context) (size uint64, checkpoint []byte, compactRange [][]byte, err error)
	StreamLeaves(ctx context.Context, start, end uint64, out chan<- logdb.StreamResult)
}

func main() {
	flag.Parse()
	ctx := context.Background()

	if len(*mysqlURI) == 0 {
		klog.Exit("Missing required parameter 'mysql_uri'")
	}
	db, err := logdb.NewDatabase(*mysqlURI)
	if err != nil {
		klog.Exitf("Failed to connect to database: %q", err)
	}

	verifier, err := note.NewVerifier("sum.golang.org+033de0ae+Ac4zctda0e5eza+HJyk9SxEdh+s3Ux18htTTAD8OuAn8")
	if err != nil {
		klog.Exitf("Failed to construct verifier: %v", err)
	}

	logVerifier := sumdbVerifier{
		db:       db,
		origin:   "go.sum database tree",
		verifier: verifier,
	}
	doit := func() {
		size, err := logVerifier.verifyLeaves(ctx)
		if err != nil {
			klog.Exitf("Failed verification: %v", err)
		}
		klog.Infof("No conflicting hashes found after verifying %d leaves", size)
	}
	doit()
	if *pollInterval == 0 {
		return
	}
	ticker := time.NewTicker(*pollInterval)
	for {
		select {
		case <-ticker.C:
			doit()
		case <-ctx.Done():
			klog.Exit(ctx.Err())
		}
	}
}

type sumdbVerifier struct {
	db       dataSource
	origin   string
	verifier note.Verifier
}

func (v sumdbVerifier) verifyLeaves(ctx context.Context) (uint64, error) {
	leaves := make(chan logdb.StreamResult, 1)

	// Get the raw data representing the latest checkpoint from the database.
	_, cpRaw, _, err := v.db.GetLatestCheckpoint(ctx)
	if err != nil {
		if err == logdb.ErrNoDataFound {
			klog.Warning("No checkpoint found in the log. Try again when the clone tool has completed.")
			return 0, nil
		}
		return 0, fmt.Errorf("GetLatestCheckpoint(): %v", err)
	}
	// Parse the checkpoint to ensure it is from the expected log.
	cp, _, _, err := log.ParseCheckpoint(cpRaw, v.origin, v.verifier)
	if err != nil {
		return 0, fmt.Errorf("ParseCheckpoint(): %v", err)
	}

	// Start streaming the leaves from the database, in order, from the beginning.
	go v.db.StreamLeaves(ctx, 0, cp.Size, leaves)

	// modVerToHashes is a map used to perform the core claim verification.
	// Two entries in the log being mapped to the same key means that the log has
	// the same module+version occurring more than once. This is only OK if both of
	// the leaf entries commit to the same hashes for this key.
	modVerToHashes := make(map[string]hashesAtIndex)

	// Construct a compact range, which is essentially an efficient in-memory Merkle Tree
	// calculator as we use it here. Every time we process a leaf we will append it to the
	// compact range, and then at the end we must check that the calculated Merkle Tree
	// root hash is the same as that in the checkpoint we parsed above.
	rf := compact.RangeFactory{
		Hash: rfc6962.DefaultHasher.HashChildren,
	}
	cr := rf.NewEmptyRange(0)

	// Now loop over each of the leaves, checking:
	// 1. Each leaf is correctly formatted (syntax)
	// 2. Each leaf is semantically valid in isolation
	// 3. That any previous declaration for the module+version is consistent with this leaf
	var resErr error
	var index uint64
	for leaf := range leaves {
		if leaf.Err != nil {
			return 0, fmt.Errorf("failed to get leaves from DB: %w", leaf.Err)
		}
		data := leaf.Leaf
		if err := cr.Append(rfc6962.DefaultHasher.HashLeaf(data), nil); err != nil {
			return 0, err
		}

		lines := strings.Split(string(data), "\n")

		line0Parts := line0RE.FindStringSubmatch(lines[0])
		line0Module, line0Version, line0Hash := line0Parts[1], line0Parts[2], line0Parts[3]

		line1Parts := line1RE.FindStringSubmatch(lines[1])
		line1Module, line1Version, line1Hash := line1Parts[1], line1Parts[2], line1Parts[3]

		if line0Module != line1Module {
			return 0, fmt.Errorf("mismatched module names at %d: (%s, %s)", index, line0Module, line1Module)
		}
		if line0Version != line1Version {
			return 0, fmt.Errorf("mismatched version names at %d: (%s, %s)", index, line0Version, line1Version)
		}

		modVer := fmt.Sprintf("%s %s", line0Module, line0Version)
		hashes := hashesAtIndex{
			line0Hash: line0Hash,
			line1Hash: line1Hash,
			index:     index,
		}

		if existing, found := modVerToHashes[modVer]; found {
			klog.V(1).Infof("Found existing hash for %q", modVer)
			if !existing.hashEq(hashes) {
				resErr = fmt.Errorf("module and version %q has conflicting hashes!\n%q != %q", modVer, existing, hashes)
				klog.Error(resErr)
			}
		}
		modVerToHashes[modVer] = hashes
		index++
	}
	if resErr != nil {
		return 0, resErr
	}

	// Use the compact range to calculate the root hash and ensure it matches the checkpoint
	rootHash, err := cr.GetRootHash(nil)
	if err != nil {
		return 0, fmt.Errorf("GetRootHash(): %v", err)
	}
	if !bytes.Equal(rootHash, cp.Hash) {
		return 0, fmt.Errorf("data corruption: checkpoint from DB has hash %x but calculated hash %x from leaves", cp.Hash, rootHash)
	}
	return index, nil
}

type hashesAtIndex struct {
	line0Hash string
	line1Hash string
	index     uint64
}

func (h hashesAtIndex) String() string {
	return fmt.Sprintf("index=%d, line0Hash=%s line1Hash=%s", h.index, h.line0Hash, h.line1Hash)
}

func (h hashesAtIndex) hashEq(other hashesAtIndex) bool {
	return h.line0Hash == other.line0Hash && h.line1Hash == other.line1Hash
}
