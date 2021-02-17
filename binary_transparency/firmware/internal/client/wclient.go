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

package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"google.golang.org/grpc/status"
)

// WitnessClient is an HTTP client for the FT witness.
type WitnessClient struct {
	// URL is the base URL for the FT witness.
	URL *url.URL
}

// GetWitnessCheckpoint returns a checkpoint from witness server
func (c WitnessClient) GetWitnessCheckpoint() (*api.LogCheckpoint, error) {
	u, err := c.URL.Parse(api.WitnessGetCheckpoint)
	if err != nil {
		return nil, err
	}
	r, err := http.Get(u.String())
	if err != nil {
		return nil, err
	}
	if r.StatusCode != 200 {
		return &api.LogCheckpoint{}, errFromRsp("failed to fetch checkpoint", r)
	}

	var wcp api.LogCheckpoint
	if err := json.NewDecoder(r.Body).Decode(&wcp); err != nil {
		return nil, err
	}
	// TODO(yd): Check signature, when it is added
	return &wcp, nil
}

func errFromRsp(m string, r *http.Response) error {
	if r.StatusCode == 200 {
		return nil
	}

	b, _ := ioutil.ReadAll(r.Body) // Ignore any error, we want to ensure we return the right status code which we already know.

	msg := fmt.Sprintf("%s: %s", m, string(b))
	return status.New(codeFromHTTPResponse(r.StatusCode), msg).Err()
}
