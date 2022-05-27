// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package download provides a fetcher to get data via http(s).
package download

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
)

// NewHTTPFetcher returns an HTTPFetcher that gets paths appended to the given prefix.
func NewHTTPFetcher(prefix *url.URL) HTTPFetcher {
	return HTTPFetcher{
		baseURL: prefix,
	}
}

// HTTPFetcher gets the data over HTTP(S).
// This is thread safe.
type HTTPFetcher struct {
	baseURL *url.URL
}

// GetData gets the data at the given path.
func (f HTTPFetcher) GetData(path string) ([]byte, error) {
	u, err := f.baseURL.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("url (%s) failed to parse(%s): %w", f.baseURL, path, err)
	}
	target := u.String()
	resp, err := http.Get(target)
	if err != nil {
		return nil, fmt.Errorf("http.Get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		// Could be worth returning a special error when we've been explicitly told to back off.
		return nil, fmt.Errorf("GET %v: %v", target, resp.Status)
	}
	// TODO(mhutchinson): Consider using io.LimitReader and making it configurable.
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ioutil.ReadAll: %w", err)
	}
	return data, nil
}
