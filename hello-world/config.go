// Copyright 2019 Google LLC
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

package helloworld

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"

	tc "github.com/google/trillian/client"
	"github.com/google/trillian/merkle/rfc6962"
)

// TreeVerifier returns a verifier configured for the log.
func TreeVerifier() (*tc.LogVerifier, error) {
	pk, err := getTrillianPK()
	if err != nil {
		return nil, fmt.Errorf("failed to load Trillian public key: %v", err)
	}

	return tc.NewLogVerifier(rfc6962.DefaultHasher, *pk, crypto.SHA256), nil
}

func getTrillianPK() (*crypto.PublicKey, error) {
	// go run github.com/google/trillian/cmd/get_tree_public_key --admin_server=localhost:50054 --log_id=7096100506408595348
	trillianPubKey := []byte(`
-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE+pbtTMrfsRbWFol03vinCGvTShba
QqnOpHiFk2BDryX/HMTL9bx2zV0kF91a8xgUKVEtDKWwPARdFvveeq8ZLg==
-----END PUBLIC KEY-----`)

	block, _ := pem.Decode(trillianPubKey)
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, errors.New("failed to decode PEM block containing public key")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	pk := pub.(crypto.PublicKey)
	return &pk, nil
}
