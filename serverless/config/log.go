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

// Package config provides the descriptor structs and example configs for
// the different entities. This allows for a common description of logs,
// witnesses, etc.
package config

import (
	"errors"
	"fmt"
	"net/url"
)

type Log struct {
	// ID is the user-chosen string used to refer to the log.
	// This may be different across witnesses, distributors, etc.
	// TODO(mhutchinson): This should be removed and ID should be hash of PK/Origin.
	ID string `yaml:"ID"`
	// PublicKey used to verify checkpoints from this log.
	PublicKey string `yaml:"PublicKey"`
	// Origin is the expected first line of checkpoints from the log.
	Origin string `yaml:"Origin"`
	// URL is the URL of the root of the log.
	// This is optional if direct log communication is not required.
	URL string `yaml:"URL"`
}

func (l Log) Validate() error {
	if l.ID == "" {
		return errors.New("missing field: ID")
	}
	if l.PublicKey == "" {
		return errors.New("missing field: PublicKey")
	}
	if l.Origin == "" {
		return errors.New("missing field: Origin")
	}
	if l.URL != "" {
		if _, err := url.Parse(l.URL); err != nil {
			return fmt.Errorf("unparseable URL: %v", err)
		}
	}
	return nil
}
