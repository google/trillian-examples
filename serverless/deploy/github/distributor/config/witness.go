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

// Package config declares how witnesses are configured.
package config

import (
	"fmt"
	"net/url"
)

// Witness describes a witness in a config file.
type Witness struct {
	// URL is the URL of the root of the witness.
	// This is optional if direct witness communication is not required.
	URL string `yaml:"URL"`
}

// Validate checks that the witness configuration is valid.
func (w Witness) Validate() error {
	if w.URL != "" {
		if _, err := url.Parse(w.URL); err != nil {
			return fmt.Errorf("unparseable URL: %v", err)
		}
	}
	return nil
}
