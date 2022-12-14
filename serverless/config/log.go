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

	"github.com/google/trillian-examples/formats/log"
)

// Log describes a verifiable log in a config file.
type Log struct {
	// ID is used to refer to the log in directory paths.
	// This field should not be manually set in configs, instead it will be
	// derived automatically by logfmt.ID.
	ID string `yaml:"ID"`
	// PublicKey used to verify checkpoints from this log.
	PublicKey string `yaml:"PublicKey"`
	// PublicKeyType identifies the format of the key present in the PublicKey field.
	// If unset, the key should be assumed to be in a format which `note.NewVerifier`
	// understands.
	PublicKeyType string `yaml:"PublicKeyType"`
	// Origin is the expected first line of checkpoints from the log.
	Origin string `yaml:"Origin"`
	// URL is the URL of the root of the log.
	// This is optional if direct log communication is not required.
	URL string `yaml:"URL"`
}

// Validate checks that the log configuration is valid.
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

// UnmarshalYAML populates the log from yaml using the unmarshal func provided.
func (l *Log) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawLog Log
	raw := rawLog{}
	if err := unmarshal(&raw); err != nil {
		return err
	}

	if len(raw.ID) > 0 {
		return errors.New("the ID field should not be manually configured")
	}
	raw.ID = log.ID(raw.Origin, []byte(raw.PublicKey))

	*l = Log(raw)
	return nil
}
