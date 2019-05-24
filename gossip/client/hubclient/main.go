// Copyright 2019 Google Inc. All Rights Reserved.
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

// hubclient is a command-line utility for interacting with Gossip Hubs.
package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/golang/glog"
	"github.com/google/certificate-transparency-go/jsonclient"
	"github.com/google/trillian"
	"github.com/google/trillian-examples/gossip/api"
	"github.com/google/trillian-examples/gossip/client"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/merkle/rfc6962"

	ct "github.com/google/certificate-transparency-go"
	cttls "github.com/google/certificate-transparency-go/tls"
)

var (
	skipHTTPSVerify = flag.Bool("skip_https_verify", false, "Skip verification of HTTPS transport connection")
	hubURI          = flag.String("hub_uri", "https://ct-gossip.sandbox.google.com/gamut", "Hub base URI")
	pubKey          = flag.String("pub_key", "", "Name of file containing hub's public key")
	timestamp       = flag.Int64("timestamp", 0, "Timestamp to use for inclusion checking (in nanosecs since Unix epoch)")
	source          = flag.String("source", "", "Source log to look for")
	getFirst        = flag.Int64("first", -1, "First entry to get")
	getLast         = flag.Int64("last", -1, "Last entry to get")
	expectCT        = flag.Bool("expect_ct", true, "Whether to expect hub entries to be RFC6962 STHs")
	expectNote      = flag.Bool("expect_note", false, "Whether to expect hub entries to be signed notes")
	treeSize        = flag.Int64("size", -1, "Tree size to query at")
	treeHash        = flag.String("tree_hash", "", "Tree hash to check against (as hex string or base64)")
	prevSize        = flag.Int64("prev_size", -1, "Previous tree size to get consistency against")
	prevHash        = flag.String("prev_hash", "", "Previous tree hash to check against (as hex string or base64)")
	leafHash        = flag.String("leaf_hash", "", "Leaf hash to retrieve (as hex string or base64)")
)

// nolint: gocyclo
func main() {
	flag.Parse()
	ctx := context.Background()

	var tlsCfg *tls.Config
	if *skipHTTPSVerify {
		glog.Warning("Skipping HTTPS connection verification")
		tlsCfg = &tls.Config{InsecureSkipVerify: *skipHTTPSVerify}
	}
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSHandshakeTimeout:   30 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			MaxIdleConnsPerHost:   10,
			DisableKeepAlives:     false,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			TLSClientConfig:       tlsCfg,
		},
	}
	opts := jsonclient.Options{UserAgent: "ct-go-hubclient/1.0"}
	if *pubKey != "" {
		pubkey, err := ioutil.ReadFile(*pubKey)
		exitOnError(err)
		opts.PublicKey = string(pubkey)
	}

	glog.V(1).Infof("Use Gossip Hub at %s", *hubURI)
	client, err := client.New(*hubURI, httpClient, opts)
	if err != nil {
		glog.Exit(err)
	}

	args := flag.Args()
	if len(args) != 1 {
		dieWithUsage("Need command argument")
	}
	switch cmd := args[0]; cmd {
	case "sth":
		getSTH(ctx, client)
	case "srcs":
		getSources(ctx, client)
	case "entries":
		if *getFirst == -1 {
			glog.Exit("No -first option supplied")
		}
		if *getLast == -1 {
			*getLast = *getFirst
		}
		getEntries(ctx, client, *getFirst, *getLast, *expectCT, *expectNote)
	case "inclusion":
		if len(*leafHash) == 0 {
			glog.Exit("No -leaf_hash option supplied")
		}
		hash, err := hashFromString(*leafHash)
		if err != nil {
			glog.Exitf("Invalid -leaf_hash supplied: %v", err)
		}
		if len(hash) != sha256.Size {
			glog.Exitf("Incorrect size (%d)for leaf hash", len(hash))
		}
		getInclusionProof(ctx, client, hash, *treeSize)
	case "consistency":
		if *treeSize <= 0 {
			glog.Exit("No valid --size supplied")
		}
		if *prevSize <= 0 {
			glog.Exit("No valid --prev_size supplied")
		}
		var hash1, hash2 []byte
		if *prevHash != "" {
			var err error
			hash1, err = hashFromString(*prevHash)
			if err != nil {
				glog.Exitf("Invalid --prev_hash: %v", err)
			}
		}
		if *treeHash != "" {
			var err error
			hash2, err = hashFromString(*treeHash)
			if err != nil {
				glog.Exitf("Invalid --tree_hash: %v", err)
			}
		}
		if (hash1 != nil) != (hash2 != nil) {
			glog.Exitf("Need both --prev_hash and --tree_hash or neither")
		}
		getConsistencyProof(ctx, client, *prevSize, *treeSize, hash1, hash2)
	case "bisect":
		if *timestamp == 0 {
			glog.Exit("No -timestamp option supplied")
		}
		findTimestamp(ctx, client, *timestamp, *expectCT)
	case "latest":
		if len(*source) == 0 {
			glog.Exitf("No -source option supplied")
		}
		getLatest(ctx, client, *source, *expectCT, *expectNote)
	default:
		dieWithUsage(fmt.Sprintf("Unknown command '%s'", cmd))
	}
}

func dieWithUsage(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	fmt.Fprintf(os.Stderr, "Usage: hubclient [options] <cmd>\n"+
		"where cmd is one of:\n"+
		"   sth           retrieve signed tree head\n"+
		"   srcs          show accepted sources\n"+
		"   entries       get hub entries (needs -first and -last)\n"+
		"   inclusion     get inclusion proof (needs -leaf_hash and optionally -size)\n"+
		"   consistency   get consistency proof (needs -size and -prev_size, optionally -tree_hash and -prev_hash)\n"+
		"   bisect        find log entry by timestamp (needs -timestamp)\n"+
		"   latest        find (best-effort) latest entry for source (needs -source)\n")
	os.Exit(1)
}

func getSTH(ctx context.Context, client *client.HubClient) {
	sth, err := client.GetSTH(ctx)
	exitOnError(err)

	when := hubTimestampToTime(sth.TreeHead.Timestamp)
	fmt.Printf("%v (timestamp: %d): Got STH for hub (size: %d) at %v, hash %x\n", when, sth.TreeHead.Timestamp, sth.TreeHead.TreeSize, client.BaseURI(), sth.TreeHead.RootHash)
	fmt.Printf("%x\n", sth.HubSignature)
}

func getSources(ctx context.Context, client *client.HubClient) {
	srcs, err := client.GetSourceKeys(ctx)
	exitOnError(err)
	for i, src := range srcs {
		fmt.Printf("%d: %q kind: %s pubKey: %x\n", i, src.ID, src.Kind, src.PubKey)
	}
}

func getEntries(ctx context.Context, client *client.HubClient, first, last int64, expectCT, expectNote bool) {
	entries, err := client.GetEntries(ctx, first, last)
	exitOnError(err)

	for i, entry := range entries {
		showEntry(entry, first+int64(i), expectCT, expectNote)
	}
}

func showEntry(entry *api.TimestampedEntry, idx int64, expectCT, expectNote bool) {
	when := hubTimestampToTime(entry.HubTimestamp)
	fmt.Printf("Index: %d Timestamp: %d (%v) Source: %q\n", idx, entry.HubTimestamp, when, entry.SourceID)
	fmt.Printf("  Data: %x\n", entry.BlobData)
	if expectNote {
		fmt.Printf("  Data as string:\n%s", string(entry.BlobData))
	}
	fmt.Printf("  Sig: %x\n", entry.SourceSignature)
	data, err := cttls.Marshal(*entry)
	if err != nil {
		glog.Errorf("failed to create marshal hub leaf: %v", err)
		return
	}
	hasher, err := hashers.NewLogHasher(trillian.HashStrategy_RFC6962_SHA256)
	if err != nil {
		glog.Exitf("failed to create hasher: %v", err)
	}
	merkleHash := hasher.HashLeaf(data)
	fmt.Printf("  MerkleLeafHash: %x\n", merkleHash)

	if !expectCT {
		return
	}
	sth, err := client.STHFromEntry(entry)
	if err != nil {
		glog.Warningf("Failed to parse entry as STH: %v", err)
		return
	}
	whenSTH := ct.TimestampToTime(sth.Timestamp)
	fmt.Printf("    Parsed as RFC6962 STH: %v (timestamp: %d): version: %v size: %d hash: %x\n", whenSTH, sth.Timestamp, sth.Version, sth.TreeSize, sth.SHA256RootHash)
}

func getInclusionProof(ctx context.Context, client *client.HubClient, hash []byte, size int64) {
	var sth *api.SignedHubTreeHead
	if size <= 0 {
		// Use the latest STH for inclusion proof.
		var err error
		sth, err = client.GetSTH(ctx)
		exitOnError(err)
		size = int64(sth.TreeHead.TreeSize)
	}
	rsp, err := client.GetProofByHash(ctx, hash, uint64(size))
	exitOnError(err)
	fmt.Printf("Inclusion proof for index %d in tree of size %d:\n", rsp.LeafIndex, size)
	for _, e := range rsp.AuditPath {
		fmt.Printf("  %x\n", e)
	}
	if sth != nil {
		// If we retrieved an STH we can verify the proof.
		verifier := merkle.NewLogVerifier(rfc6962.DefaultHasher)
		if err := verifier.VerifyInclusionProof(rsp.LeafIndex, int64(sth.TreeHead.TreeSize), rsp.AuditPath, sth.TreeHead.RootHash, hash); err != nil {
			glog.Exitf("Failed to VerifyInclusionProof(%d, %d)=%v", rsp.LeafIndex, sth.TreeHead.TreeSize, err)
		}
		fmt.Printf("Verified that hash %x + proof = root hash %x\n", hash, sth.TreeHead.RootHash)
	}
}

func getConsistencyProof(ctx context.Context, client *client.HubClient, first, second int64, prevHash, treeHash []byte) {
	proof, err := client.GetSTHConsistency(ctx, uint64(first), uint64(second))
	exitOnError(err)
	fmt.Printf("Consistency proof from size %d to size %d:\n", first, second)
	for _, e := range proof {
		fmt.Printf("  %x\n", e)
	}
	if prevHash == nil || treeHash == nil {
		return
	}
	// We have tree hashes so we can verify the proof.
	verifier := merkle.NewLogVerifier(rfc6962.DefaultHasher)
	if err := verifier.VerifyConsistencyProof(first, second, prevHash, treeHash, proof); err != nil {
		glog.Exitf("Failed to VerifyConsistencyProof(%x @size=%d, %x @size=%d): %v", prevHash, first, treeHash, second, err)
	}
	fmt.Printf("Verified that hash %x @%d + proof = hash %x @%d\n", prevHash, first, treeHash, second)
}

func findTimestamp(ctx context.Context, client *client.HubClient, target int64, expectCT bool) {
	sth, err := client.GetSTH(ctx)
	exitOnError(err)
	getEntry := func(idx int64) *api.TimestampedEntry {
		entries, err := client.GetEntries(ctx, idx, idx)
		exitOnError(err)
		if l := len(entries); l != 1 {
			glog.Exitf("Unexpected number (%d) of entries received requesting index %d", l, idx)
		}
		return entries[0]
	}
	// Performing a binary search assumes that the timestamps are
	// monotonically increasing.
	idx := sort.Search(int(sth.TreeHead.TreeSize), func(idx int) bool {
		glog.V(1).Infof("check timestamp at index %d", idx)
		entry := getEntry(int64(idx))
		return (entry.HubTimestamp >= uint64(target))
	})
	when := hubTimestampToTime(uint64(target))
	if idx >= int(sth.TreeHead.TreeSize) {
		fmt.Printf("No entry with timestamp>=%d (%v) found up to tree size %d\n", target, when, sth.TreeHead.TreeSize)
		return
	}
	fmt.Printf("First entry with timestamp>=%d (%v) found at index %d\n", target, when, idx)
	getEntries(ctx, client, int64(idx), int64(idx), expectCT, false)
}

func getLatest(ctx context.Context, client *client.HubClient, source string, expectCT, expectNote bool) {
	entry, err := client.GetLatestForSource(ctx, source)
	exitOnError(err)
	showEntry(entry, -1, expectCT, expectNote) // no index available
}

func exitOnError(err error) {
	if err == nil {
		return
	}
	if err, ok := err.(jsonclient.RspError); ok {
		glog.Infof("HTTP details: status=%d, body:\n%s", err.StatusCode, err.Body)
	}
	glog.Exit(err.Error())
}

func hashFromString(input string) ([]byte, error) {
	hash, err := hex.DecodeString(input)
	if err == nil && len(hash) == sha256.Size {
		return hash, nil
	}
	hash, err = base64.StdEncoding.DecodeString(input)
	if err == nil && len(hash) == sha256.Size {
		return hash, nil
	}
	return nil, fmt.Errorf("hash value %q failed to parse as 32-byte hex or base64", input)
}

func hubTimestampToTime(ts uint64) time.Time {
	return time.Unix(0, int64(ts))
}
