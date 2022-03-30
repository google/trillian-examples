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

// http is a simple client for interacting with witnesses over HTTP.
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"

	wit_api "github.com/google/trillian-examples/witness/golang/api"
)

// ErrCheckpointTooOld is returned if the checkpoint passed to Update needs to be updated.
var ErrCheckpointTooOld error = errors.New("checkpoint too old")

func NewWitness(url *url.URL) Witness {
	return Witness{
		url:    url,
		client: *http.DefaultClient, // TODO(mhutchinson): Make this an arg
	}
}

// Witness is a simple client for interacting with witnesses over HTTP.
type Witness struct {
	url    *url.URL
	client http.Client
}

// GetLatestCheckpoint returns a recent checkpoint from the witness for the specified log ID.
func (w Witness) GetLatestCheckpoint(ctx context.Context, logID string) ([]byte, error) {
	u, err := w.url.Parse(fmt.Sprintf(wit_api.HTTPGetCheckpoint, logID))
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %v", err)
	}
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	resp, err := w.client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to do http request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, os.ErrNotExist
	} else if resp.StatusCode != 200 {
		return nil, fmt.Errorf("bad status response: %s", resp.Status)
	}
	return ioutil.ReadAll(resp.Body)
}

// Update attempts to clock the witness forward for the given logID.
// The latest signed checkpoint will be returned if this succeeds, or if the error is
// http.ErrCheckpointTooOld. In all other cases no checkpoint should be expected.
func (w Witness) Update(ctx context.Context, logID string, cp []byte, proof [][]byte) ([]byte, error) {
	reqBody, err := json.MarshalIndent(&wit_api.UpdateRequest{
		Checkpoint: cp,
		Proof:      proof,
	}, "", " ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal update request: %v", err)
	}
	u, err := w.url.Parse(fmt.Sprintf(wit_api.HTTPUpdate, logID))
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %v", err)
	}
	req, err := http.NewRequest("PUT", u.String(), bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	resp, err := w.client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to do http request: %v", err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %v", err)
	}
	if resp.StatusCode != 200 {
		if resp.StatusCode == 409 {
			return body, fmt.Errorf("%w: %s", ErrCheckpointTooOld, resp.Status)
		}
		return nil, fmt.Errorf("bad status response (%s): %q", resp.Status, body)
	}
	return body, nil
}
