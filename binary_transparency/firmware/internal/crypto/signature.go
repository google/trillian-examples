// Copyright 2020 Google LLC. All Rights Reserved.
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

package crypto

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"strings"
)

func getPrivateKey() (*rsa.PrivateKey, error) {
	mKey := strings.NewReader(RSApri)
	priv, err := ioutil.ReadAll(mKey)
	if err != nil {
		return nil, fmt.Errorf("read failed! %s", err)
	}

	privPem, rest := pem.Decode(priv)
	if len(rest) != 0 {
		return nil, fmt.Errorf("extraneous data: %v", rest)
	}
	if privPem == nil {
		return nil, fmt.Errorf("pem decoded to nil")
	}
	if privPem.Type != "RSA PRIVATE KEY" {
		return nil, fmt.Errorf("RSA private key is of the wrong type %s", privPem.Type)
	}

	var privateKey *rsa.PrivateKey
	if privateKey, err = x509.ParsePKCS1PrivateKey(privPem.Bytes); err != nil {
		return nil, fmt.Errorf("unable to parse RSA private key %v", err)
	}

	return privateKey, nil
}

func getPublicKey() (*rsa.PublicKey, error) {
	mKey := strings.NewReader(RSApub)
	pub, err := ioutil.ReadAll(mKey)
	if err != nil {
		return nil, fmt.Errorf("public key read failed! %s", err)
	}

	pubPem, rest := pem.Decode(pub)
	if len(rest) != 0 {
		return nil, fmt.Errorf("extraneous data: %v", rest)
	}
	if pubPem == nil {
		return nil, fmt.Errorf("pem decoded to nil")
	}
	if pubPem.Type != "RSA PUBLIC KEY" {
		return nil, fmt.Errorf("RSA public key is of the wrong type %s", pubPem.Type)
	}

	var pubKey *rsa.PublicKey
	if pubKey, err = x509.ParsePKCS1PublicKey(pubPem.Bytes); err != nil {
		return nil, fmt.Errorf("unable to parse RSA public key %v", err)
	}

	return pubKey, nil
}

//SignMessage is used to sign the Statement
func SignMessage(msg []byte) ([]byte, error) {
	// Before signing, we need to hash the message
	// The hash is what we actually sign
	h := sha512.Sum512(msg)

	// Get the required key for signing
	key, err := getPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("private key fetch failed %v", err)
	}
	// use PSS over PKCS#1 v1.5 for enhanced security
	signature, err := rsa.SignPSS(rand.Reader, key, crypto.SHA512, h[:], nil)
	if err != nil {
		return nil, fmt.Errorf("signing failed for firmware statement %v", err)
	}
	return signature, nil
}

//VerifySignature is used to verify the incoming message
func VerifySignature(msg []byte, signature []byte) error {
	// Get the required key for signing
	key, err := getPublicKey()
	if err != nil {
		return fmt.Errorf("public key fetch failed %v", err)
	}
	// Before verify, we need to hash the message
	// The hash is what we actually verify
	h := sha512.Sum512(msg)

	if err = rsa.VerifyPSS(key, crypto.SHA512, h[:], signature, nil); err != nil {
		return fmt.Errorf("failed to verify signature %v", err)
	}
	// If we don't get any error from the `VerifyPSS` method, implies our
	// signature is valid
	return nil
}
