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
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/serverless/api"
	"github.com/google/trillian-examples/serverless/internal/client"
	"github.com/google/trillian/merkle/logverifier"
	"github.com/google/trillian/merkle/rfc6962/hasher"
)

var (
	storageURL = flag.String("storage_url", "", "Log storage root URL, e.g. file:///path/to/log or https://log.server/and/path")
)

func usage() {
	fmt.Fprintf(os.Stderr, "Please specify one of the commands and its arguments:\n")
	fmt.Fprintf(os.Stderr, "  inclusion <file> [index-in-log]\n")
	os.Exit(-1)
}

func main() {
	flag.Parse()

	if len(*storageURL) == 0 {
		glog.Exitf("--storage_url must be provided")
	}

	rootURL, err := url.Parse(*storageURL)
	if err != nil {
		glog.Exitf("Invalid storage URL: %q", err)
	}
	f := newFetcher(rootURL)
	logState, err := client.GetLogState(f)
	if err != nil {
		glog.Exitf("Failed to fetch log state: %q", err)
	}

	args := flag.Args()
	if len(args) == 0 {
		usage()
	}
	switch args[0] {
	case "inclusion":
		err = inclusionProof(*logState, f, args[1:])
	default:
		usage()
	}
	if err != nil {
		glog.Exitf("Command %q failed: %q", args[0], err)
	}
}

func inclusionProof(state api.LogState, f client.FetcherFunc, args []string) error {
	if l := len(args); l < 1 || l > 2 {
		return fmt.Errorf("usage: inclusion <file> [index-in-log]")
	}
	entry, err := ioutil.ReadFile(args[0])
	if err != nil {
		return fmt.Errorf("failed to read entry from %q: %w", args[0], err)
	}

	h := hasher.DefaultHasher
	lh := h.HashLeaf(entry)

	var idx uint64
	if len(args) == 2 {
		idx, err = strconv.ParseUint(args[1], 16, 64)
		if err != nil {
			return fmt.Errorf("invalid index-in-log %q: %w", args[1], err)
		}
	} else {
		idx, err = client.LookupIndex(f, lh)
		if err != nil {
			return fmt.Errorf("failed to lookup leaf index: %w", err)
		}
		glog.Infof("Leaf %q found at index %d", args[0], idx)
	}

	builder, err := client.NewProofBuilder(state, h.HashChildren, f)
	if err != nil {
		return fmt.Errorf("failed to create proof builder: %w", err)
	}

	proof, err := builder.InclusionProof(idx)
	if err != nil {
		return fmt.Errorf("failed to get inclusion proof: %w", err)
	}

	glog.V(1).Infof("Built inclusion proof: %#x", proof)

	lv := logverifier.New(hasher.DefaultHasher)
	if err := lv.VerifyInclusionProof(int64(idx), int64(state.Size), proof, state.RootHash, lh); err != nil {
		return fmt.Errorf("failed to verify inclusion proof: %q", err)
	}

	glog.Infof("Inclusion verified in tree size %d, with root 0x%0x", state.Size, state.RootHash)
	return nil
}

// newFetcher creates a FetcherFunc for the log at the given root location.
func newFetcher(root *url.URL) client.FetcherFunc {
	get := getByScheme[root.Scheme]
	if get == nil {
		panic(fmt.Errorf("unsupported URL scheme %s", root.Scheme))
	}

	return func(p string) ([]byte, error) {
		u, err := root.Parse(p)
		if err != nil {
			return nil, err
		}
		return get(u)
	}
}

var getByScheme = map[string]func(*url.URL) ([]byte, error){
	"http":  readHTTP,
	"https": readHTTP,
	"file": func(u *url.URL) ([]byte, error) {
		return ioutil.ReadFile(u.Path)
	},
}

func readHTTP(u *url.URL) ([]byte, error) {
	resp, err := http.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)
}
