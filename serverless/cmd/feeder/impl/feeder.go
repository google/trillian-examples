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

// impl is a witness feeder implementation for the serverless log.
package impl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/serverless/client"
	"github.com/google/trillian/merkle/rfc6962/hasher"
	"golang.org/x/mod/sumdb/note"

	wit_api "github.com/google/trillian-examples/witness/golang/api"
)

// WitnessConfig encapulates all the config needed for a witness.
type WitnessConfig struct {
	// Name is a human readable name for the witness.
	Name string `json:"name"`
	// URL is the root of the witness' HTTP API.
	URL string `json:"url"`
	// PublicKey is the witness' public key.
	PublicKey string `json:"public_key"`
}

// Config encapsulates the feeder config.
type Config struct {
	// The LogID used by the witnesses to identify this log.
	LogID string `json:"log_id"`
	// PublicKey associated with LogID.
	LogPublicKey string `json:"log_public_key"`
	// LogURL is the URL of the root of the log.
	LogURL string `json:"log_url"`
	// Witnesses is a list of all configured witnesses.
	Witnesses []WitnessConfig `json:"witnesses"`
	// NumRequired is the minimum number of cosignatures required for a feeding run
	// to be considered successful.
	NumRequired int `json:"num_required"`
}

// Witness sends the provided checkpoint to the configured set of witnesses.
// Returns the provided checkpoint plus at least cfg.NumRequired signatures.
func Witness(ctx context.Context, cp []byte, logFetcher client.Fetcher, cfg Config) ([]byte, error) {
	logSigV, err := note.NewVerifier(cfg.LogPublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create log signature verifier: %v", err)
	}

	n, err := note.Open(cp, note.VerifierList(logSigV))
	if err != nil {
		return nil, fmt.Errorf("failed to verify signature on checkpoint: %v", err)
	}
	cpSubmit := &log.Checkpoint{}
	_, err = cpSubmit.Unmarshal([]byte(n.Text))
	if err != nil {
		return nil, fmt.Errorf("failed to verify signature on checkpoint to witness: %v", err)
	}

	sigs := make(chan note.Signature, len(cfg.Witnesses))
	errs := make(chan error, len(cfg.Witnesses))
	// TODO(al): make this configurable
	h := hasher.DefaultHasher

	for _, w := range cfg.Witnesses {
		go func(ctx context.Context) {
			wSigV, err := note.NewVerifier(w.PublicKey)
			if err != nil {
				errs <- fmt.Errorf("%s: invalid witness publickey: %v", w.Name, err)
				return
			}

			// Keep submitting until success or context timeout...
			t := time.NewTicker(1)
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					t.Reset(5 * time.Second)
				}

				latestCP, sig, err := fetchLatestCP(ctx, w.URL, cfg.LogID, wSigV)
				if err != nil {
					glog.Warningf("%s: failed to fetch latest CP: %v", w.Name, err)
					continue
				}

				if latestCP.Size > cpSubmit.Size {
					errs <- fmt.Errorf("%s: witness checkpoint size (%d) > submit checkpoint size (%d)", w.Name, latestCP.Size, cpSubmit.Size)
					return
				}
				if latestCP.Size == cpSubmit.Size && bytes.Equal(latestCP.Hash, cpSubmit.Hash) {
					sigs <- *sig
					return
				}

				pb, err := client.NewProofBuilder(ctx, *cpSubmit, h.HashChildren, logFetcher)
				if err != nil {
					glog.Warning("%s: failed to create proof builder: %v", w.Name, err)
					continue
				}

				conP, err := pb.ConsistencyProof(ctx, latestCP.Size, cpSubmit.Size)
				if err != nil {
					glog.Warning("%s: failed to build consistency proof: %v", w.Name, err)
					continue
				}

				if err := submitCP(ctx, cp, conP, cfg.LogID, w); err != nil {
					glog.Warning("%s: failed to submit checkpoint to witness: %v", w.Name, err)
					continue

				}

			}
		}(ctx)
	}

	for range cfg.Witnesses {
		select {
		case s := <-sigs:
			n.Sigs = append(n.Sigs, s)
		case e := <-errs:
			glog.Warning(e.Error())
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if got := len(n.Sigs); got < cfg.NumRequired {
		return nil, fmt.Errorf("number of witness signatures (%d) < number required (%d)", got, cfg.NumRequired)
	}
	return note.Sign(n)
}

func fetchLatestCP(ctx context.Context, witURL, logID string, wSigV note.Verifier) (*log.Checkpoint, *note.Signature, error) {
	url := fmt.Sprintf("%s/%s", witURL, fmt.Sprintf(wit_api.HTTPGetCheckpoint, logID))

	hReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create new HTTP request: %v", err)
	}
	resp, err := http.DefaultClient.Do(hReq.WithContext(ctx))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch %q: %v", url, err)
	}
	if resp.StatusCode != 200 {
		return nil, nil, fmt.Errorf("got bad response from witness %q: %v", witURL, resp.Status)
	}
	defer resp.Body.Close()
	raw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read body from %q: %v", url, err)
	}
	n, err := note.Open(raw, note.VerifierList(wSigV))
	if err != nil {
		return nil, nil, fmt.Errorf("%s: failed to validate CP signature: %v", witURL, err)
	}
	cp := &log.Checkpoint{}
	_, err = cp.Unmarshal([]byte(n.Text))
	if err != nil {
		glog.Infof("Bad checkpoint from %q (%v):\n", witURL, err, n.Text)
		return nil, nil, fmt.Errorf("failed to unmarshal checkpoint from")
	}

	return cp, &n.Sigs[0], nil
}

func submitCP(ctx context.Context, cp []byte, proof [][]byte, logID string, wCfg WitnessConfig) error {
	req := wit_api.UpdateRequest{
		Checkpoint: cp,
		Proof:      proof,
	}
	body, err := json.MarshalIndent(&req, "", " ")
	if err != nil {
		return fmt.Errorf("failed to marshal request: %v", err)
	}

	url := fmt.Sprintf("%s/witness/v0/logs/%s/update", wCfg.URL, logID)

	hReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to build HTTP request to %q: %v", wCfg.Name, err)
	}
	resp, err := http.DefaultClient.Do(hReq.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("failed to send HTTP request to %q: %v", wCfg.Name, err)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("got bad response from witness %q: %v", wCfg.Name, resp.Status)
	}

	return nil
}
