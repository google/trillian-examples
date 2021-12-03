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

// Package serverless is an implementation of a witness feeder for serverless logs.
package serverless

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/internal/feeder"
	"github.com/google/trillian-examples/serverless/client"
	"github.com/google/trillian-examples/serverless/config"
	"github.com/transparency-dev/merkle/rfc6962"

	i_note "github.com/google/trillian-examples/internal/note"
	wit_http "github.com/google/trillian-examples/witness/golang/client/http"
)

// FeedLog periodically feeds checkpoints from the log to the witness.
// This function returns once the provided context is done.
func FeedLog(ctx context.Context, l config.Log, w wit_http.Witness, timeout time.Duration, interval time.Duration) error {
	lURL, err := url.Parse(l.URL)
	if err != nil {
		return fmt.Errorf("invalid LogURL %q: %v", l.URL, err)
	}
	logSigV, err := i_note.NewVerifier(l.PublicKeyType, l.PublicKey)
	if err != nil {
		return err
	}
	f := newFetcher(lURL)
	h := rfc6962.DefaultHasher

	fetchCP := func(ctx context.Context) ([]byte, error) {
		return f(ctx, "checkpoint")
	}
	fetchProof := func(ctx context.Context, from, to log.Checkpoint) ([][]byte, error) {
		if from.Size == 0 {
			return [][]byte{}, nil
		}
		pb, err := client.NewProofBuilder(ctx, to, h.HashChildren, f)
		if err != nil {
			return nil, fmt.Errorf("failed to create proof builder for %q: %v", l.Origin, err)
		}

		conP, err := pb.ConsistencyProof(ctx, from.Size, to.Size)
		if err != nil {
			return nil, fmt.Errorf("failed to create proof for %q(%d -> %d): %v", l.Origin, from.Size, to.Size, err)
		}
		return conP, nil
	}

	opts := feeder.FeedOpts{
		LogID:           l.ID,
		LogOrigin:       l.Origin,
		FetchCheckpoint: fetchCP,
		FetchProof:      fetchProof,
		LogSigVerifier:  logSigV,
		Witness:         w,
	}
	if interval > 0 {
		return feeder.Run(context.Background(), interval, opts)
	}
	_, err = feeder.FeedOnce(context.Background(), opts)
	return err
}

// TODO(al): factor this stuff out and share between tools:

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
