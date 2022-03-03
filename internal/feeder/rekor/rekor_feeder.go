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

// Package rekor is an implementation of a witness feeder for the Sigstore log: Rekór.
package rekor

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/internal/feeder"
	"github.com/google/trillian-examples/serverless/config"

	i_note "github.com/google/trillian-examples/internal/note"
)

// logInfo is a partial representation of the JSON object returned by Rekór's
// api/v1/log request.
type logInfo struct {
	// SignedTreeHead contains a Rekór checkpoint.
	SignedTreeHead string `json:"signedTreeHead"`
}

// proof is a partial representation of the JSON struct returned by the Rekór
// api/v1/log/proof request.
type proof struct {
	Hashes []string `json:"hashes"`
}

// FeedLog feeds checkpoints from the source log to the witness.
// If interval is non-zero, this function will return when the context is done, otherwise it will perform
// one feed cycle and return.
func FeedLog(ctx context.Context, l config.Log, w feeder.Witness, c *http.Client, interval time.Duration) error {
	lURL, err := url.Parse(l.URL)
	if err != nil {
		return fmt.Errorf("invalid LogURL %q: %v", l.URL, err)
	}
	logSigV, err := i_note.NewVerifier(l.PublicKeyType, l.PublicKey)
	if err != nil {
		return err
	}

	fetchCP := func(ctx context.Context) ([]byte, error) {
		li := logInfo{}
		if err := getJSON(ctx, c, lURL, "api/v1/log", &li); err != nil {
			return nil, fmt.Errorf("failed to fetch log info: %v", err)
		}
		return []byte(li.SignedTreeHead), nil
	}
	fetchProof := func(ctx context.Context, from, to log.Checkpoint) ([][]byte, error) {
		if from.Size == 0 {
			return [][]byte{}, nil
		}
		cp := proof{}
		if err := getJSON(ctx, c, lURL, fmt.Sprintf("api/v1/log/proof?firstSize=%d&lastSize=%d", from.Size, to.Size), &cp); err != nil {
			return nil, fmt.Errorf("failed to fetch log info: %v", err)
		}
		var err error
		p := make([][]byte, len(cp.Hashes))
		for i := range cp.Hashes {
			p[i], err = hex.DecodeString(cp.Hashes[i])
			if err != nil {
				return nil, fmt.Errorf("invalid proof element at %d: %v", i, err)
			}
		}
		return p, nil
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
		return feeder.Run(ctx, interval, opts)
	}
	_, err = feeder.FeedOnce(ctx, opts)
	return err
}

func getJSON(ctx context.Context, c *http.Client, base *url.URL, path string, s interface{}) error {
	u, err := base.Parse(path)
	if err != nil {
		return fmt.Errorf("failed to parse URL: %v", err)
	}
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}
	req = req.WithContext(ctx)

	rsp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request to %q: %v", u.String(), err)
	}
	defer rsp.Body.Close()

	if rsp.StatusCode == 404 {
		return os.ErrNotExist
	}
	if rsp.StatusCode != 200 {
		return fmt.Errorf("unexpected status fetching %q: %s", u.String(), rsp.Status)
	}

	raw, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return fmt.Errorf("failed to read body from %q: %v", u.String(), err)
	}
	if err := json.Unmarshal(raw, s); err != nil {
		glog.Infof("Got body:\n%s", string(raw))
		return fmt.Errorf("failed to unmarshal JSON: %v", err)
	}
	return nil
}
