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
	"io/ioutil"
	"testing"

	"gopkg.in/yaml.v3"
)

type exampleLogConfig struct {
	Log Log `yaml:"Log"`
}

func TestExampleConfig(t *testing.T) {
	bs, err := ioutil.ReadFile("example_log_config.yaml")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	config := exampleLogConfig{}
	if err := yaml.Unmarshal(bs, &config); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if err := config.Log.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestConfigOverrideID(t *testing.T) {
	bs, err := ioutil.ReadFile("example_log_config.yaml")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	bs = append(bs, []byte("\n\tID: wibble")...)
	config := exampleLogConfig{}
	if err := yaml.Unmarshal(bs, &config); err == nil {
		t.Fatal("Unmarshal expected error, but got none")
	}
}
