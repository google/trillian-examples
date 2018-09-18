// Copyright 2018 Google Inc. All Rights Reserved.
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

package hub

import (
	"context"
	"crypto"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian-examples/gossip/hub/configpb"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/monitoring"
)

// ConfigFromSingleFile creates a HubMultiConfig proto from the given
// filename, which should contain text-protobuf encoded configuration data
// for a hub configuration without backend information (i.e. a HubConfigSet).
// Does not do full validation of the config but checks that it is non empty.
func ConfigFromSingleFile(filename, beSpec string) (*configpb.HubMultiConfig, error) {
	cfgText, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var cfg configpb.HubConfigSet
	if err := proto.UnmarshalText(string(cfgText), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse hub config: %v", err)
	}
	if len(cfg.Config) == 0 {
		return nil, errors.New("empty hub config found")
	}

	defaultBackend := &configpb.HubBackend{Name: "default", BackendSpec: beSpec}
	for _, c := range cfg.Config {
		c.HubBackendName = defaultBackend.Name
	}
	return &configpb.HubMultiConfig{
		Backends:   &configpb.HubBackendSet{Backend: []*configpb.HubBackend{defaultBackend}},
		HubConfigs: &configpb.HubConfigSet{Config: cfg.Config},
	}, nil
}

// ConfigFromMultiFile creates a HubMultiConfig proto from the given
// filename, which should contain text-protobuf encoded configuration data
// for a multi-backend hub configuration (i.e. a HubMultiConfig).
// Does not do full validation of the config but checks that it is non empty.
func ConfigFromMultiFile(filename string) (*configpb.HubMultiConfig, error) {
	cfgText, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var cfg configpb.HubMultiConfig
	if err := proto.UnmarshalText(string(cfgText), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse multi-backend hub config: %v", err)
	}

	if len(cfg.HubConfigs.GetConfig()) == 0 || len(cfg.Backends.GetBackend()) == 0 {
		return nil, errors.New("config is missing backends and/or hub configs")
	}
	return &cfg, nil
}

// ValidateHubMultiConfig checks that a config is valid for use with multiple
// backend Trillian log servers. The rules applied are:
//
// 1. The backend set must define a set of hub backends with distinct
// (non empty) names and non empty backend specs.
// 2. The backend specs must all be distinct.
// 3. The hub configs must all specify a hub backend and each must be one of
// those defined in the backend set.
// 4. The prefixes of configured hubs must all be distinct and must not be
// empty.
// 5. The set of tree ids for each configured backend must be distinct.
func ValidateHubMultiConfig(cfg *configpb.HubMultiConfig) (map[string]*configpb.HubBackend, error) {
	// Check the backends have unique non empty names and build the map.
	backendMap := make(map[string]*configpb.HubBackend)
	bSpecMap := make(map[string]bool)
	for _, backend := range cfg.Backends.Backend {
		if len(backend.Name) == 0 {
			return nil, fmt.Errorf("empty backend name: %v", backend)
		}
		if len(backend.BackendSpec) == 0 {
			return nil, fmt.Errorf("empty backend_spec for backend: %v", backend)
		}
		if _, ok := backendMap[backend.Name]; ok {
			return nil, fmt.Errorf("duplicate backend name: %v", backend)
		}
		if ok := bSpecMap[backend.BackendSpec]; ok {
			return nil, fmt.Errorf("duplicate backend spec: %v", backend)
		}
		backendMap[backend.Name] = backend
		bSpecMap[backend.BackendSpec] = true
	}

	// Check that hubs all reference a defined backend and there are no duplicate
	// or empty prefixes. Apply other HubConfig specific checks.
	hubNameMap := make(map[string]bool)
	hubIDMap := make(map[string]bool)
	for _, hubCfg := range cfg.HubConfigs.Config {
		if len(hubCfg.Prefix) == 0 {
			return nil, fmt.Errorf("hub config: empty prefix: %v", hubCfg)
		}
		if hubNameMap[hubCfg.Prefix] {
			return nil, fmt.Errorf("hub config: duplicate prefix: %s: %v", hubCfg.Prefix, hubCfg)
		}
		if _, ok := backendMap[hubCfg.HubBackendName]; !ok {
			return nil, fmt.Errorf("hub config: references undefined backend: %s: %v", hubCfg.HubBackendName, hubCfg)
		}
		hubNameMap[hubCfg.Prefix] = true
		hubIDKey := fmt.Sprintf("%s-%d", hubCfg.HubBackendName, hubCfg.LogId)
		if ok := hubIDMap[hubIDKey]; ok {
			return nil, fmt.Errorf("hub config: dup tree id: %d for: %v", hubCfg.LogId, hubCfg)
		}
		hubIDMap[hubIDKey] = true
	}

	return backendMap, nil
}

// InstanceOptions describes the options for a hub instance.
type InstanceOptions struct {
	Deadline      time.Duration
	MaxGetEntries int64
	MetricFactory monitoring.MetricFactory
	// ErrorMapper converts an error from an RPC request to an HTTP status, plus
	// a boolean to indicate whether the conversion succeeded.
	ErrorMapper func(error) (int, bool)
}

// SetUpInstance sets up a hub instance that uses the specified client to communicate
// with the Trillian RPC back end.
func SetUpInstance(ctx context.Context, client trillian.TrillianLogClient, cfg *configpb.HubConfig, opts InstanceOptions) (*PathHandlers, error) {
	// Check config validity.
	if len(cfg.Source) == 0 {
		return nil, errors.New("need to specify Source")
	}
	if cfg.PrivateKey == nil {
		return nil, errors.New("need to specify PrivateKey")
	}

	// Load the trusted source public keys.
	cryptoMap := make(map[string]sourceCryptoInfo)
	for _, src := range cfg.Source {
		if _, ok := cryptoMap[src.Id]; ok {
			return nil, fmt.Errorf("Duplicate source log entry %s for ID %s", src.Name, src.Id)
		}
		pubKey, err := x509.ParsePKIXPublicKey(src.PublicKey.Der)
		if err != nil {
			return nil, fmt.Errorf("Failed to parse public key for %s <%s>: %v", src.Name, src.Id, err)
		}
		var hasher crypto.Hash
		switch src.HashAlgorithm {
		case sigpb.DigitallySigned_SHA256:
			hasher = crypto.SHA256
		default:
			return nil, fmt.Errorf("Failed to determine hash algorithm %d", src.HashAlgorithm)
		}
		cryptoMap[src.Id] = sourceCryptoInfo{pubKeyData: src.PublicKey.Der, pubKey: pubKey, hasher: hasher}
	}

	// Load the private key for this hub.
	var keyProto ptypes.DynamicAny
	if err := ptypes.UnmarshalAny(cfg.PrivateKey, &keyProto); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cfg.PrivateKey: %v", err)
	}
	signer, err := keys.NewSigner(ctx, keyProto.Message)
	if err != nil {
		return nil, fmt.Errorf("failed to load private key: %v", err)
	}

	// Create and register the handlers using the RPC client we just set up.
	hubInfo := newHubInfo(cfg.LogId, cfg.Prefix, client, signer, cryptoMap, opts)

	handlers := hubInfo.Handlers(cfg.Prefix)
	return &handlers, nil
}
