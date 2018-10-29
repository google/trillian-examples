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

// to_proto is a utility for generating protobuf text stanzas that describe
// a given private key.
package main

import (
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/certificate-transparency-go/x509"
	"github.com/google/trillian-examples/gossip/hub/configpb"
	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/crypto/keyspb"
)

var password = flag.String("password", "", "Password for private key file(s)")

func keyDataFromPEM(keyPEM []byte, password string) ([]byte, error) {
	rest := keyPEM
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			return nil, errors.New("no private key PEM block found")
		}
		if strings.Contains(block.Type, "PRIVATE KEY") {
			if password != "" {
				derData, err := x509.DecryptPEMBlock(block, []byte(password))
				if err != nil {
					return nil, fmt.Errorf("failed to decrypt: %v", err)
				}
				return derData, nil
			}
			return block.Bytes, nil
		}
	}
}

func main() {
	flag.Parse()

	for _, arg := range flag.Args() {
		keyPEM, err := ioutil.ReadFile(arg)
		if err != nil {
			glog.Errorf("%v: Failed to read data: %v", arg, err)
			continue
		}
		keyDER, err := keyDataFromPEM(keyPEM, *password)
		if err != nil {
			glog.Errorf("%v: Invalid PEM file: %v", arg, err)
			continue
		}
		privProto, err := ptypes.MarshalAny(&keyspb.PrivateKey{Der: keyDER})
		if err != nil {
			glog.Errorf("%v: failed to marshal private key as Any: %v", arg, err)
			continue
		}
		// Parse the private key to allow public key generation.
		signer, err := der.UnmarshalPrivateKey(keyDER)
		if err != nil {
			glog.Errorf("%v: failed to parse private key: %v", arg, err)
			continue
		}
		pubDER, err := der.MarshalPublicKey(signer.Public())
		if err != nil {
			glog.Errorf("%v: failed to marshal public key: %v", arg, err)
			continue
		}

		cfg := configpb.HubConfig{
			PrivateKey: privProto,
			PublicKey:  &keyspb.PublicKey{Der: pubDER},
		}
		proto.MarshalText(os.Stdout, &cfg)
	}
}
