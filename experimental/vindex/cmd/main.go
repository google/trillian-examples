// Copyright 2025 Google LLC. All Rights Reserved.
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

// vindex builds a verifiable map in memory from a clone of a log.
package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/trillian-examples/clone/logdb"
	"github.com/gorilla/mux"
	"github.com/transparency-dev/formats/log"
	"github.com/transparency-dev/incubator/vindex"
	"golang.org/x/mod/module"
	"golang.org/x/mod/sumdb/note"
	"k8s.io/klog/v2"

	_ "github.com/go-sql-driver/mysql"
)

var (
	logDSN  = flag.String("logDSN", "", "Connection string for a clone DB log.")
	walPath = flag.String("walPath", "", "Path to use for the Write Ahead Log. If empty, a temporary file will be used.")
	addr    = flag.String("addr", ":8088", "Address to set up HTTP server listening on")

	// Example leaf:
	// golang.org/x/text v0.3.0 h1:g61tztE5qeGQ89tm6NTjjM9VPIm088od1l6aSorWRWg=
	// golang.org/x/text v0.3.0/go.mod h1:NqM8EUOU14njkJ3fqMW+pc6Ldnwhi/IjpwHt7yyuwOQ=
	//
	line0RE = regexp.MustCompile(`(.*) (.*) h1:(.*)`)
	line1RE = regexp.MustCompile(`(.*) (.*)/go.mod h1:(.*)`)
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	ctx := context.Background()

	logDB, err := logdb.NewDatabase(*logDSN)
	if err != nil {
		klog.Exitf("Failed to connect to DB: %s", err)
	}

	b, err := vindex.NewVerifiableIndex(ctx, &logDBAdapter{logDB}, logParserFromFlags(), mapFnFromFlags(), walPathFromFlags())
	if err != nil {
		klog.Exitf("NewIndexBuilder(): %v", err)
	}

	s := NewServer(func(s string) string {
		idxes := b.Lookup(s)
		return fmt.Sprintf("Indices in log: %v", idxes)

	})
	r := mux.NewRouter()
	s.registerHandlers(r)
	hServer := &http.Server{
		Addr:    *addr,
		Handler: r,
	}

	go func() {
		for {
			if err := b.Update(ctx); err != nil {
				klog.Exitf("Failed to update Verifiable Index: %s", err)
			}
			// On successful update, this should post the vindex root into an output log.
			// Log is likely to be POSIX Tessera.
			klog.Info("Verifiable Index updated")

			select {
			case <-ctx.Done():
				return
			case <-time.After(30 * time.Second):
			}
		}
	}()

	klog.Infof("Starting HTTP server listening on %s", *addr)
	if err := hServer.ListenAndServe(); err != nil {
		klog.Exit(err)
	}
}

type logDBAdapter struct {
	cloneDB *logdb.Database
}

func (a *logDBAdapter) GetCheckpoint(ctx context.Context) (checkpoint []byte, err error) {
	_, cp, _, err := a.cloneDB.GetLatestCheckpoint(ctx)
	return cp, err
}

func (a *logDBAdapter) StreamLeaves(ctx context.Context, start, end uint64, out chan<- vindex.LeafOrError) {
	inChan := make(chan logdb.StreamResult)
	go func() {
		defer close(out)

		for r := range inChan {
			out <- vindex.LeafOrError{
				Leaf:  r.Leaf,
				Error: r.Err,
			}
		}
	}()
	a.cloneDB.StreamLeaves(ctx, start, end, inChan)
}

func logParserFromFlags() func(cpRaw []byte) (*log.Checkpoint, error) {
	// TODO(mhutchinson): Implement a flag-selectable switch for which verifier to use.
	// While we only support sumDB, this is hard-coded.
	const vkey = "sum.golang.org+033de0ae+Ac4zctda0e5eza+HJyk9SxEdh+s3Ux18htTTAD8OuAn8"
	const origin = "go.sum database tree"
	verifier, err := note.NewVerifier(vkey)
	if err != nil {
		klog.Exitf("Failed to create verifier: %s", err)
	}
	return func(cpRaw []byte) (*log.Checkpoint, error) {
		// No witnesses required yet
		cp, _, _, err := log.ParseCheckpoint(cpRaw, origin, verifier)
		return cp, err
	}
}

func mapFnFromFlags() vindex.MapFn {
	// TODO(mhutchinson): Implement a flag-selectable switch for which MapFn to use.
	// Realistically, this would be multiple binaries in a real world application, but
	// for the sake of a demo, showing that it's exactly the same binary apart from the
	// MapFn is a selling point.
	mapFn := func(data []byte) [][32]byte {
		lines := strings.Split(string(data), "\n")
		if len(lines) < 2 {
			panic(fmt.Errorf("expected 2 lines but got %d", len(lines)))
		}

		line0Parts := line0RE.FindStringSubmatch(lines[0])
		line0Module, line0Version := line0Parts[1], line0Parts[2]

		line1Parts := line1RE.FindStringSubmatch(lines[1])
		line1Module, line1Version := line1Parts[1], line1Parts[2]

		if line0Module != line1Module {
			klog.Errorf("mismatched module names: (%s, %s)", line0Module, line1Module)
		}
		if line0Version != line1Version {
			klog.Errorf("mismatched version names: (%s, %s)", line0Version, line0Version)
		}
		if module.IsPseudoVersion(line0Version) {
			// Drop any emphemeral builds
			return nil
		}

		klog.V(2).Infof("MapFn found: Module: %s:\t%s", line0Module, line0Version)

		return [][32]byte{sha256.Sum256([]byte(line0Module))}
	}
	return mapFn
}

func walPathFromFlags() string {
	if len(*walPath) > 0 {
		return *walPath
	}
	f, err := os.CreateTemp("", "walPath")
	if err != nil {
		klog.Exitf("Failed to create temporary path for WAL: %s", err)
	}
	klog.Infof("Created temporary WAL at %s", f.Name())
	return f.Name()
}
