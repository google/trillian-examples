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
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/serverless/client"
	"github.com/google/trillian-examples/serverless/client/witness"
	"github.com/transparency-dev/merkle/proof"
	"github.com/transparency-dev/merkle/rfc6962"
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

// aString is a flag Value which holds multiple strings, allowing the flag to
// be specified multiple times on the command line.
type aString []string

func (a *aString) String() string {
	return fmt.Sprintf("%v", *a)
}

func (a *aString) Set(v string) error {
	*a = append(*a, v)
	return nil
}

func flagStringList(name, usage string) *aString {
	r := make(aString, 0)
	flag.Var(&r, name, usage)
	return &r
}

var (
	cacheDir            = flag.String("cache_dir", defaultCacheLocation(), "Where to cache client state for logs, if empty don't store anything locally")
	distributorURLs     = flagStringList("distributor_url", "URL identifying the root of a distributor (can specify this flag repeatedly)")
	logURL              = flag.String("log_url", "", "Log storage root URL, e.g. file:///path/to/log or https://log.server/and/path")
	logPubKeyFile       = flag.String("log_public_key", "", "Location of log public key file. If unset, uses the contents of the SERVERLESS_LOG_PUBLIC_KEY environment variable")
	logID               = flag.String("log_id", "", "LogID used by distributors. Will be derived from log public key if unset")
	origin              = flag.String("origin", "", "Expected first line of checkpoints from log")
	witnessPubKeyFiles  = flagStringList("witness_public_key", "File containing witness public key (can specify this flag repeatedly)")
	witnessSigsRequired = flag.Int("witness_sigs_required", 0, "Minimum number of witness signatures required for consensus")
	outputCheckpoint    = flag.String("output_checkpoint", "", "If set, the update command will write the latest verified consistent checkpoint to this file")
	outputConsistency   = flag.String("output_consistency_proof", "", "If set, the update and consistency commands will write the verified consistency proof used to update the checkpoint to this file")
	outputInclusion     = flag.String("output_inclusion_proof", "", "If set, the inclusion command will write the verified inclusion proof to this file")
)

func usage() {
	fmt.Fprintf(os.Stderr, "Please specify one of the commands and its arguments:\n")
	fmt.Fprintf(os.Stderr, "  consistency <from-size> <to-size>\n - build consistency proof between two log sizes\n")
	fmt.Fprintf(os.Stderr, "  inclusion <file> [index-in-log]\n - verify inclusion of a file in the log\n")
	fmt.Fprintf(os.Stderr, "  update - force the client to update its latest checkpoint\n")
	os.Exit(-1)
}

func main() {
	flag.Parse()
	ctx := context.Background()

	logSigV, pubK, err := logSigVerifier(*logPubKeyFile)
	if err != nil {
		glog.Exitf("failed to read log public key: %v", err)
	}
	logID := *logID
	if logID == "" {
		logID = log.ID(*origin, pubK)
	}

	u := *logURL
	if len(u) == 0 {
		glog.Exitf("--log_url must be provided")
	}
	// url must reference a directory, by definition
	if !strings.HasSuffix(u, "/") {
		u += "/"
	}

	rootURL, err := url.Parse(u)
	if err != nil {
		glog.Exitf("Invalid log URL: %v", err)
	}

	witnesses, err := witnessSigVerifiers(*witnessPubKeyFiles)
	if err != nil {
		glog.Exitf("Failed to read witness pub keys: %v", err)
	}

	if want, got := *witnessSigsRequired, len(witnesses); want > got {
		glog.Exitf("--witness_sigs_required=%d but only %d witnesses configured", want, got)
	}

	distribs, err := distributors()
	if err != nil {
		glog.Exitf("Failed to create distributors list: %v", err)
	}

	f := newFetcher(rootURL)
	lc, err := newLogClientTool(ctx, logID, f, logSigV, witnesses, distribs)
	if err != nil {
		glog.Exitf("Failed to create new client: %v", err)
	}

	args := flag.Args()
	if len(args) == 0 {
		usage()
	}
	switch args[0] {
	case "consistency":
		err = lc.consistencyProof(ctx, args[1:])
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
	Fetcher client.Fetcher
	Hasher  *rfc6962.Hasher
	Tracker client.LogStateTracker
}

func newLogClientTool(ctx context.Context, logID string, logFetcher client.Fetcher, logSigV note.Verifier, witnesses []note.Verifier, distributors []client.Fetcher) (*logClientTool, error) {
	var cpRaw []byte
	var err error
	if len(*cacheDir) > 0 {
		cpRaw, err = loadLocalCheckpoint(logID)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("failed to load cached checkpoint: %q", err)
		}
	} else {
		glog.Info("Local log state cache disabled")
	}

	hasher := rfc6962.DefaultHasher
	var cons client.ConsensusCheckpointFunc
	if *witnessSigsRequired == 0 {
		glog.V(1).Infof("witness_sigs_required is 0, using unilateral consensus")
		cons = client.UnilateralConsensus(logFetcher)
	} else {
		glog.V(1).Infof("witness_sigs_required > 0, using checkpoint.N consensus")
		cons, err = witness.CheckpointNConsensus(logID, distributors, witnesses, *witnessSigsRequired)
		if err != nil {
			return nil, fmt.Errorf("failed to create consensus func: %v", err)
		}
	}
	tracker, err := client.NewLogStateTracker(ctx, logFetcher, hasher, cpRaw, logSigV, *origin, cons)

	if err != nil {
		glog.Warningf("%s", string(cpRaw))
		return nil, fmt.Errorf("failed to create LogStateTracker: %q", err)
	}

	return &logClientTool{
		Fetcher: logFetcher,
		Hasher:  hasher,
		Tracker: tracker,
	}, nil
}

func (l *logClientTool) consistencyProof(ctx context.Context, args []string) error {
	if l := len(args); l != 2 {
		return fmt.Errorf("usage: consistency <from-size> <to-size>")
	}

	from, err := strconv.ParseUint(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid from-size %q: %w", args[1], err)
	}
	to, err := strconv.ParseUint(args[1], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid to-size %q: %w", args[1], err)
	}
	if from >= to {
		return errors.New("from-size must be less than to-size")
	}

	builder, err := client.NewProofBuilder(ctx, l.Tracker.LatestConsistent, l.Hasher.HashChildren, l.Fetcher)
	if err != nil {
		return fmt.Errorf("failed to create proof builder: %w", err)
	}

	p, err := builder.ConsistencyProof(ctx, from, to)
	if err != nil {
		return fmt.Errorf("failed to build consistency proof: %w", err)
	}

	glog.V(1).Infof("Built consistency proof: %#x", p)

	if o := *outputConsistency; len(o) > 0 {
		if err := os.WriteFile(o, []byte(merkleProof(p).Marshal()), 0644); err != nil {
			return fmt.Errorf("failed to write inclusion proof to %q: %v", o, err)
		}
	}
	return nil
}

func (l *logClientTool) inclusionProof(ctx context.Context, args []string) error {
	if l := len(args); l < 1 || l > 2 {
		return fmt.Errorf("usage: inclusion <file> [index-in-log]")
	}
	entry, err := os.ReadFile(args[0])
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

	p, err := builder.InclusionProof(ctx, idx)
	if err != nil {
		return fmt.Errorf("failed to get inclusion proof: %w", err)
	}

	glog.V(1).Infof("Built inclusion proof: %#x", p)

	if err := proof.VerifyInclusion(l.Hasher, idx, cp.Size, lh, p, cp.Hash); err != nil {
		return fmt.Errorf("failed to verify inclusion proof: %q", err)
	}

	if o := *outputInclusion; len(o) > 0 {
		ps := []byte(merkleProof(p).Marshal())
		if err := os.WriteFile(o, ps, 0644); err != nil {
			glog.Warningf("Failed to write inclusion proof to %q: %v", o, err)
		}
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

	_, p, newCPRaw, err := l.Tracker.Update(ctx)
	if err != nil {
		return fmt.Errorf("failed to update checkpoint: %w", err)
	}

	if o := *outputCheckpoint; len(o) > 0 {
		if err := os.WriteFile(o, newCPRaw, 0644); err != nil {
			glog.Warningf("Failed to write latest checkpint to %q: %v", o, err)
		}
	}

	if lcp := l.Tracker.LatestConsistent; lcp.Size == cp.Size {
		glog.Info("Log hasn't grown, nothing to update.")
		return nil
	}

	if o := *outputConsistency; len(o) > 0 {
		if err := os.WriteFile(o, []byte(merkleProof(p).Marshal()), 0644); err != nil {
			glog.Warningf("Failed to write consistency proof to %q: %v", o, err)
		}
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
		return os.ReadFile(u.Path)
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
	switch resp.StatusCode {
	case 404:
		glog.Infof("Not found: %q", u.String())
		return nil, os.ErrNotExist
	case 200:
		break
	default:
		return nil, fmt.Errorf("unexpected http status %q", resp.Status)
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// loadLocalCheckpoint reads the serialised checkpoint for the given logID from the
// local client cache.
func loadLocalCheckpoint(logID string) ([]byte, error) {
	cpPath := filepath.Join(*cacheDir, logID, "checkpoint")
	return os.ReadFile(cpPath)
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
	if err := os.WriteFile(cpPathTmp, cpRaw, 0644); err != nil {
		return err
	}
	return os.Rename(cpPathTmp, cpPath)
}

// Returns a log signature verifier and the public key bytes it uses.
// Attempts to read key material from f, or uses the SERVERLESS_LOG_PUBLIC_KEY
// env var if f is unset.
func logSigVerifier(f string) (note.Verifier, []byte, error) {
	var pubKey []byte
	var err error
	if len(f) > 0 {
		pubKey, err = os.ReadFile(f)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read public key from file %q: %v", f, err)
		}
	} else {
		pubKey = []byte(os.Getenv("SERVERLESS_LOG_PUBLIC_KEY"))
		if len(pubKey) == 0 {
			return nil, nil, fmt.Errorf("supply public key file path using --log_public_key or set SERVERLESS_LOG_PUBLIC_KEY environment variable")
		}
	}

	v, err := note.NewVerifier(string(pubKey))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create verifier: %v", err)
	}

	return v, pubKey, nil
}

func witnessSigVerifiers(fs []string) ([]note.Verifier, error) {
	vs := make([]note.Verifier, 0, len(fs))
	for _, f := range fs {
		v, err := sigVerifierFromFile(f)
		if err != nil {
			return nil, err
		}
		glog.V(1).Infof("Found witness %q", v.Name())
		vs = append(vs, v)
	}
	glog.V(1).Infof("Found %d witnesses", len(vs))
	return vs, nil
}

func sigVerifierFromFile(f string) (note.Verifier, error) {
	k, err := os.ReadFile(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read public key from file %q: %v", f, err)
	}
	return note.NewVerifier(string(k))
}

func distributors() ([]client.Fetcher, error) {
	distribs := make([]client.Fetcher, 0)
	for _, d := range *distributorURLs {
		u, err := url.Parse(d)
		if err != nil {
			return nil, fmt.Errorf("invalid distributor URL %q: %v", d, err)
		}
		distribs = append(distribs, newFetcher(u))
	}
	return distribs, nil
}

// merkleProof represents Merkle proofs.
type merkleProof [][]byte

// Marshal returns a simple string-based representation of the proof.
func (p merkleProof) Marshal() string {
	b := strings.Builder{}
	for _, l := range p {
		b.WriteString(base64.StdEncoding.EncodeToString(l))
		b.WriteRune('\n')
	}
	return b.String()
}
