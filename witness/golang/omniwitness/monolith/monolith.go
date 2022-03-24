// Copyright 2022 Google LLC. All Rights Reserved.
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

// monolith is a single executable that runs all of the feeders and witness
// in a single process.
package main

import (
	"context"
	"flag"
	"net"
	"net/http"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/witness/golang/omniwitness/internal/omniwitness"
	"golang.org/x/mod/sumdb/note"
)

var (
	addr = flag.String("listen", ":8080", "Address to listen on")

	signingKey  = flag.String("private_key", "", "The note-compatible signing key to use")
	verifierKey = flag.String("public_key", "", "The note-compatible verifier key to use")

	githubUser  = flag.String("gh_user", "", "The github user account to propose witnessed PRs from")
	githubEmail = flag.String("gh_email", "", "The email that witnessed checkopoint git commits should be done under")
	githubToken = flag.String("gh_token", "", "The github auth token to allow checkpoint distribution via PRs")

	httpTimeout = flag.Duration("http_timeout", 10*time.Second, "HTTP timeout for outbound requests")
)

func main() {
	flag.Parse()
	ctx := context.Background()

	httpListener, err := net.Listen("tcp", *addr)
	if err != nil {
		glog.Fatalf("failed to listen on %q", *addr)
	}
	httpClient := &http.Client{
		Timeout: *httpTimeout,
	}

	signer, err := note.NewSigner(*signingKey)
	if err != nil {
		glog.Exitf("Failed to init signer: %v", err)
	}
	verifier, err := note.NewVerifier(*verifierKey)
	if err != nil {
		glog.Exitf("Failed to init verifier: %v", err)
	}
	opConfig := omniwitness.OperatorConfig{
		WitnessSigner:   signer,
		WitnessVerifier: verifier,

		GithubUser:  *githubUser,
		GithubEmail: *githubEmail,
		GithubToken: *githubToken,
	}
	if err := omniwitness.Main(ctx, opConfig, httpListener, httpClient); err != nil {
		glog.Exitf("Main failed: %v", err)
	}
}
