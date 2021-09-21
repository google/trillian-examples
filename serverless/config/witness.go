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

package config

import (
	"errors"
	"fmt"
	"net/url"
)

type Witness struct {
	// PublicKey used to verify checkpoints are signed by this witness.
	PublicKey string `yaml:"PublicKey"`
	// URL is the URL of the root of the witness.
	// This is optional if direct witness communication is not required.
	URL string `yaml:"URL"`
}

func (w Witness) Validate() error {
	if w.PublicKey == "" {
		return errors.New("missing field: PublicKey")
	}
	if w.URL != "" {
		if _, err := url.Parse(w.URL); err != nil {
			return fmt.Errorf("unparseable URL: %v", err)
		}
	}
	return nil
}
