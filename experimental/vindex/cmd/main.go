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
	"errors"
	"flag"
	"fmt"
	"iter"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/google/trillian-examples/clone/logdb"
	"github.com/gorilla/mux"
	"github.com/transparency-dev/formats/log"
	fnote "github.com/transparency-dev/formats/note"
	"github.com/transparency-dev/incubator/vindex"
	"golang.org/x/mod/module"
	"golang.org/x/mod/sumdb/note"
	"k8s.io/klog/v2"

	_ "github.com/go-sql-driver/mysql"
)

var (
	logDSN               = flag.String("logDSN", "", "Connection string for a clone DB log.")
	storageDir           = flag.String("storage_dir", "", "Root directory in which to store the data for the demo. This will create subdirectories for the Output Log, and allocate space to store the verifiable map persistence.")
	listen               = flag.String("addr", ":8088", "Address to set up HTTP server listening on")
	outputLogPrivKeyFile = flag.String("output_log_private_key", "", "Location of private key file. If unset, uses the contents of the OUTPUT_LOG_PRIVATE_KEY environment variable.")

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

	if err := run(ctx); err != nil {
		klog.Exitf("Run failed: %v", err)
	}
}

func run(ctx context.Context) error {
	if *storageDir == "" {
		return errors.New("storage_dir must be set")
	}
	outputLogDir := path.Join(*storageDir, "outputlog")
	mapRoot := path.Join(*storageDir, "vindex")

	if err := os.MkdirAll(outputLogDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output log directory: %v", err)
	}
	if err := os.MkdirAll(mapRoot, 0o755); err != nil {
		return fmt.Errorf("failed to create vindex directory: %v", err)
	}

	logDB, err := logdb.NewDatabase(*logDSN)
	if err != nil {
		return fmt.Errorf("failed to connect to DB: %s", err)
	}

	const vkey = "sum.golang.org+033de0ae+Ac4zctda0e5eza+HJyk9SxEdh+s3Ux18htTTAD8OuAn8"
	const origin = "go.sum database tree"
	verifier, err := note.NewVerifier(vkey)
	if err != nil {
		klog.Exitf("Failed to create verifier: %s", err)
	}

	inputLog := &logDBAdapter{
		cloneDB: logDB,
		v:       verifier,
		origin:  origin,
	}
	outputLog, outputCloser := outputLogOrDie(ctx, outputLogDir)
	defer outputCloser()

	vi, err := vindex.NewVerifiableIndex(ctx, inputLog, mapFnFromFlags(), outputLog, mapRoot)
	if err != nil {
		return fmt.Errorf("failed to build verifiable index: %v", err)
	}

	// Keeps the map synced with the latest published log state.
	go maintainMap(ctx, vi)

	// Run a web server to handle queries over the verifiable index.
	go runWebServer(vi, outputLogDir)
	<-ctx.Done()
	return nil
}

type logDBAdapter struct {
	cloneDB *logdb.Database
	v       note.Verifier
	origin  string
}

func (a *logDBAdapter) Checkpoint(ctx context.Context) (checkpoint []byte, err error) {
	_, cp, _, err := a.cloneDB.GetLatestCheckpoint(ctx)
	return cp, err
}

func (a *logDBAdapter) Leaves(ctx context.Context, start, end uint64) iter.Seq2[[]byte, error] {
	inChan := make(chan logdb.StreamResult)
	go a.cloneDB.StreamLeaves(ctx, start, end, inChan)
	return func(yield func([]byte, error) bool) {
		var r logdb.StreamResult
		var ok bool
		for {
			select {
			case <-ctx.Done():
				// Context cancelled, yield the context error and stop.
				// This handles termination due to external cancellation.
				if !yield(nil, ctx.Err()) {
					return // Consumer stopped
				}
				return // Producer stopped
			case r, ok = <-inChan:
				if !ok {
					return // Channel closed by the sender
				}
			}
			if !yield(r.Leaf, r.Err) {
				return // Consumer stopped
			}
		}
	}
}

func (a *logDBAdapter) Parse(cpRaw []byte) (*log.Checkpoint, error) {
	cp, _, _, err := log.ParseCheckpoint(cpRaw, a.origin, a.v)
	return cp, err
}

// outputLogOrDie returns an output log using a POSIX log in the given directory.
func outputLogOrDie(ctx context.Context, outputLogDir string) (log vindex.OutputLog, closer func()) {
	s, v := getOutputLogSignerVerifierOrDie()

	l, c, err := vindex.NewOutputLog(ctx, outputLogDir, s, v)
	if err != nil {
		klog.Exit(err)
	}
	return l, c
}

// Read output log private key from file or environment variable and generate the
// note Signer and Verifier pair for it.
func getOutputLogSignerVerifierOrDie() (note.Signer, note.Verifier) {
	var privKey string
	var err error
	if len(*outputLogPrivKeyFile) > 0 {
		privKey, err = getKeyFile(*outputLogPrivKeyFile)
		if err != nil {
			klog.Exitf("Unable to get private key: %v", err)
		}
	} else {
		privKey = os.Getenv("OUTPUT_LOG_PRIVATE_KEY")
		if len(privKey) == 0 {
			klog.Exit("Supply private key file path using --output_log_private_key or set OUTPUT_LOG_PRIVATE_KEY environment variable")
		}
	}
	s, v, err := fnote.NewEd25519SignerVerifier(privKey)
	if err != nil {
		klog.Exitf("Failed to get signer/verifier: %v", err)
	}
	return s, v
}

func getKeyFile(path string) (string, error) {
	k, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read key file: %w", err)
	}
	return string(k), nil
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

func runWebServer(vi *vindex.VerifiableIndex, outLogDir string) {
	web := NewServer(func(h [sha256.Size]byte) ([]uint64, error) {
		idxes, size := vi.Lookup(h)
		if size == 0 {
			return nil, errors.New("index not populated")
		}
		return idxes, nil
	})

	olfs := http.FileServer(http.Dir(outLogDir))
	r := mux.NewRouter()
	r.PathPrefix("/outputlog/").Handler(http.StripPrefix("/outputlog/", olfs))
	web.registerHandlers(r)
	hServer := &http.Server{
		Addr:    *listen,
		Handler: r,
	}
	go func() {
		_ = hServer.ListenAndServe()
	}()
	klog.Infof("Started HTTP server listening on %s", *listen)
}

// maintainMap reads entries from the log and sync them to the vindex.
func maintainMap(ctx context.Context, vi *vindex.VerifiableIndex) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		if err := vi.Update(ctx); err != nil {
			klog.Warning(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
