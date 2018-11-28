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

// The make_hub_cfg binary creates a Hub configuration and a collection of
// private keys for fake source Logs tracked by the Hub.
package main

import (
	"bufio"
	"encoding/pem"
	"flag"
	"fmt"
	"os"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/certificate-transparency-go/x509"
	"github.com/google/trillian-examples/gossip/hub/configpb"
	"github.com/google/trillian-examples/gossip/integration"
)

var (
	hubConfig        = flag.String("hub_config", "", "File to emit Hub configuration into")
	logPrivKeyPrefix = flag.String("log_priv_key_prefix", "", "Prefix for files holding source log private keys")
	hubCount         = flag.Int("hubs", 3, "Number of hub instances to emit")
	logCount         = flag.Int("logs", 3, "Number of source logs tracked by each hub")
)

func main() {
	flag.Parse()
	if len(*hubConfig) == 0 {
		glog.Exitf("No --hub_config output destination specified")
	}
	if len(*logPrivKeyPrefix) == 0 {
		glog.Exitf("No --log_priv_key_prefix specified")
	}
	cfgs, logKeys, err := integration.BuildTestConfig(*hubCount, *logCount)
	if err != nil {
		glog.Exitf("Failed to build test config: %v", err)
	}

	// Build and emit a single-backend Hub configuration.
	hubCfg := configpb.HubConfigSet{Config: cfgs}
	for _, cfg := range hubCfg.Config {
		// Set a special LogId value to allow easy replacement with a real tree ID.
		cfg.LogId = 999999
	}

	cfgFile, err := os.Create(*hubConfig)
	if err != nil {
		glog.Exitf("Failed to open %q for writing: %v", *hubConfig, err)
	}
	defer cfgFile.Close()
	w := bufio.NewWriter(cfgFile)
	if err := proto.MarshalText(w, &hubCfg); err != nil {
		glog.Exitf("Failed to marshal hub config: %v", err)
	}
	w.Flush()

	// Emit the private keys.
	for i, logKey := range logKeys {
		filename := fmt.Sprintf("%s-%02d.pem", *logPrivKeyPrefix, i)
		keyFile, err := os.Create(filename)
		if err != nil {
			glog.Exitf("Failed to open %q for writing: %v", filename, err)
		}
		w := bufio.NewWriter(keyFile)

		derKey, err := x509.MarshalECPrivateKey(logKey)
		if err != nil {
			glog.Exitf("Failed to marshal source log private key: %v", err)
		}
		pem.Encode(w, &pem.Block{Type: "PRIVATE KEY", Bytes: derKey})
		w.Flush()
		keyFile.Close()
	}
}
