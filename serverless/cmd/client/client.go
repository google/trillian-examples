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

// client is a read-only cli for interacting with serverless logs.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/serverless/client"
	"github.com/google/trillian/merkle/logverifier"
	"github.com/google/trillian/merkle/rfc6962"
	"golang.org/x/mod/sumdb/note"
)

func defaultCacheLocation() string {
	hd, err := os.UserCacheDir()
	if err != nil {
		glog.Warningf("Failed to determine user cache dir: %q", err)
		return ""
	}
	return fmt.Sprintf("%s/serverless", hd)
}

var (
	logURL     = flag.String("log_url", "", "Log storage root URL, e.g. file:///path/to/log or https://log.server/and/path")
	cacheDir   = flag.String("cache_dir", defaultCacheLocation(), "Where to cache client state for logs, if empty don't store anything locally.")
	pubKeyFile = flag.String("public_key", "", "Location of public key file. If unset, uses the contents of the SERVERLESS_LOG_PUBLIC_KEY environment variable.")
)

func usage() {
	fmt.Fprintf(os.Stderr, "Please specify one of the commands and its arguments:\n")
	fmt.Fprintf(os.Stderr, "  inclusion <file> [index-in-log]\n - verify inclusion of a file in the log\n")
	fmt.Fprintf(os.Stderr, "  update - force the client to update its latest checkpoint\n")
	os.Exit(-1)
}

func main() {
	flag.Parse()
	ctx := context.Background()

	// Read log public key from file or environment variable
	var pubKey string
	if len(*pubKeyFile) > 0 {
		k, err := ioutil.ReadFile(*pubKeyFile)
		if err != nil {
			glog.Exitf("failed to read public_key file: %q", err)
		}
		pubKey = string(k)
	} else {
		pubKey = os.Getenv("SERVERLESS_LOG_PUBLIC_KEY")
		if len(pubKey) == 0 {
			glog.Exit("supply public key file path using --public_key or set SERVERLESS_LOG_PUBLIC_KEY environment variable")
		}
	}

	if len(*logURL) == 0 {
		glog.Exitf("--log_url must be provided")
	}

	rootURL, err := url.Parse(*logURL)
	if err != nil {
		glog.Exitf("Invalid log URL: %q", err)
	}

	// Derive logID from log public key
	v, err := note.NewVerifier(pubKey)
	if err != nil {
		glog.Exitf("Failed to instantiate Verifier : %q", err)
	}
	logID := fmt.Sprintf("%d", v.KeyHash())

	f := newFetcher(rootURL)
	lc, err := newLogClientTool(ctx, logID, f, pubKey)
	if err != nil {
		glog.Exitf("Failed to create new client: %q", err)
	}

	args := flag.Args()
	if len(args) == 0 {
		usage()
	}
	switch args[0] {
	case "inclusion":
		err = lc.inclusionProof(ctx, args[1:])
	case "update":
		err = lc.updateCheckpoint(ctx, args[1:])
	default:
		usage()
	}
	if err != nil {
		glog.Exitf("Command %q failed: %q", args[0], err)
	}

	// Persist new view of log state, if required.
	if len(*cacheDir) > 0 {
		if err := storeLocalCheckpoint(logID, lc.Tracker.LatestConsistentRaw); err != nil {
			glog.Exitf("Failed to persist local log state: %q", err)
		}
	}
}

// logClientTool encapsulates the "application level" interaction with the log.
// It relies heavily on the components provided by the `internal/client` package
// to accomplish this.
type logClientTool struct {
	Fetcher  client.Fetcher
	Hasher   *rfc6962.Hasher
	Verifier logverifier.LogVerifier
	Tracker  client.LogStateTracker
}

func newLogClientTool(ctx context.Context, logID string, f client.Fetcher, pubKey string) (logClientTool, error) {
	var cpRaw []byte
	var err error
	if len(*cacheDir) > 0 {
		cpRaw, err = loadLocalCheckpoint(logID)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			glog.Exitf("Failed to load cached checkpoint: %q", err)
		}
	} else {
		glog.Info("Local log state cache disabled")
	}

	hasher := rfc6962.DefaultHasher
	lv := logverifier.New(hasher)
	v, err := note.NewVerifier(pubKey)
	if err != nil {
		glog.Exitf("Failed to instantiate Verifier: %q", err)
	}
	tracker, err := client.NewLogStateTracker(ctx, f, hasher, cpRaw, v)
	if err != nil {
		glog.Exitf("Failed to create LogStateTracker: %q", err)
	}

	return logClientTool{
		Fetcher:  f,
		Hasher:   hasher,
		Verifier: lv,
		Tracker:  tracker,
	}, nil
}

func (l *logClientTool) inclusionProof(ctx context.Context, args []string) error {
	if l := len(args); l < 1 || l > 2 {
		return fmt.Errorf("usage: inclusion <file> [index-in-log]")
	}
	entry, err := ioutil.ReadFile(args[0])
	if err != nil {
		return fmt.Errorf("failed to read entry from %q: %w", args[0], err)
	}
	lh := l.Hasher.HashLeaf(entry)

	var idx uint64
	if len(args) == 2 {
		idx, err = strconv.ParseUint(args[1], 16, 64)
		if err != nil {
			return fmt.Errorf("invalid index-in-log %q: %w", args[1], err)
		}
	} else {
		idx, err = client.LookupIndex(ctx, l.Fetcher, lh)
		if err != nil {
			return fmt.Errorf("failed to lookup leaf index: %w", err)
		}
		glog.Infof("Leaf %q found at index %d", args[0], idx)
	}

	// TODO(al): wait for growth if necessary

	cp := l.Tracker.LatestConsistent
	builder, err := client.NewProofBuilder(ctx, cp, l.Hasher.HashChildren, l.Fetcher)
	if err != nil {
		return fmt.Errorf("failed to create proof builder: %w", err)
	}

	proof, err := builder.InclusionProof(ctx, idx)
	if err != nil {
		return fmt.Errorf("failed to get inclusion proof: %w", err)
	}

	glog.V(1).Infof("Built inclusion proof: %#x", proof)

	if err := l.Verifier.VerifyInclusionProof(int64(idx), int64(cp.Size), proof, cp.Hash, lh); err != nil {
		return fmt.Errorf("failed to verify inclusion proof: %q", err)
	}

	glog.Infof("Inclusion verified under checkpoint:\n%s", cp.Marshal())
	return nil
}

func (l *logClientTool) updateCheckpoint(ctx context.Context, args []string) error {
	if l := len(args); l != 0 {
		return fmt.Errorf("usage: update")
	}

	glog.V(1).Infof("Original checkpoint:\n%s", l.Tracker.LatestConsistentRaw)
	cp := l.Tracker.LatestConsistent

	if err := l.Tracker.Update(ctx); err != nil {
		return fmt.Errorf("failed to update checkpoint: %w", err)
	}

	if lcp := l.Tracker.LatestConsistent; lcp.Size == cp.Size {
		glog.Info("Log hasn't grown, nothing to update.")
		return nil
	}

	glog.Infof("Updated checkpoint:\n%s", l.Tracker.LatestConsistentRaw)

	return nil
}

// newFetcher creates a Fetcher for the log at the given root location.
func newFetcher(root *url.URL) client.Fetcher {
	get := getByScheme[root.Scheme]
	if get == nil {
		panic(fmt.Errorf("unsupported URL scheme %s", root.Scheme))
	}

	return func(ctx context.Context, p string) ([]byte, error) {
		u, err := root.Parse(p)
		if err != nil {
			return nil, err
		}
		return get(ctx, u)
	}
}

var getByScheme = map[string]func(context.Context, *url.URL) ([]byte, error){
	"http":  readHTTP,
	"https": readHTTP,
	"file": func(_ context.Context, u *url.URL) ([]byte, error) {
		return ioutil.ReadFile(u.Path)
	},
}

func readHTTP(ctx context.Context, u *url.URL) ([]byte, error) {
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)
}

// loadLocalCheckpoint reads the serialised checkpoint for the given logID from the
// local client cache.
func loadLocalCheckpoint(logID string) ([]byte, error) {
	cpPath := filepath.Join(*cacheDir, logID, "checkpoint")
	return ioutil.ReadFile(cpPath)
}

// storeLocalCheckpoint updates the local client cache for the specified log with
// the provided serialised log checkpoint.
func storeLocalCheckpoint(logID string, cpRaw []byte) error {
	cpDir := filepath.Join(*cacheDir, logID)
	if err := os.MkdirAll(cpDir, 0700); err != nil {
		return err
	}
	cpPath := filepath.Join(cpDir, "checkpoint")
	cpPathTmp := fmt.Sprintf("%s.tmp", cpPath)
	if err := ioutil.WriteFile(cpPathTmp, cpRaw, 0644); err != nil {
		return err
	}
	return os.Rename(cpPathTmp, cpPath)
}
