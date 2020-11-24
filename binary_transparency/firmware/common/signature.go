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

package common

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"strings"

	"github.com/golang/glog"
)

func getPrivateKey() (*rsa.PrivateKey, error) {
	mKey := strings.NewReader(RSA_pri)
	priv, err := ioutil.ReadAll(mKey)
	if err != nil {
		glog.Exitf("Read failed! %s", err)
	}

	privPem, _ := pem.Decode(priv)
	var privPemBytes []byte
	if privPem.Type != "RSA PRIVATE KEY" {
		glog.Exitf("RSA private key is of the wrong type %s", privPem.Type)
	}
	privPemBytes = privPem.Bytes

	var parsedKey interface{}
	if parsedKey, err = x509.ParsePKCS1PrivateKey(privPemBytes); err != nil {
		if parsedKey, err = x509.ParsePKCS8PrivateKey(privPemBytes); err != nil {
			glog.Exitf("Unable to parse RSA private key %v", err)
		}
	}

	var privateKey *rsa.PrivateKey
	var ok bool
	privateKey, ok = parsedKey.(*rsa.PrivateKey)
	if !ok {
		glog.Exitf("Unable to parse RSA private key %v", err)
	}
	return privateKey, nil
}

func getPublicKey() (*rsa.PublicKey, error) {
	mKey := strings.NewReader(RSA_pub)
	pub, err := ioutil.ReadAll(mKey)
	if err != nil {
		glog.Exitf("Pubkey read failed! %s", err)
	}

	pubPem, _ := pem.Decode(pub)
	if pubPem == nil {
		glog.Exit("pem decoded to nil")
	}
	if pubPem.Type != "RSA PUBLIC KEY" {
		glog.Exitf("RSA public key is of the wrong type %s", pubPem.Type)
	}

	var parsedKey interface{}
	if parsedKey, err = x509.ParsePKCS1PublicKey(pubPem.Bytes); err != nil {
		glog.Exitf("Unable to parse RSA public key, generating a temp one %v", err)
	}

	var pubKey *rsa.PublicKey
	var ok bool
	if pubKey, ok = parsedKey.(*rsa.PublicKey); !ok {
		glog.Exit("Unable to parse RSA public key")
	}

	return pubKey, nil
}

//SignMessage is used to sign the Statement
func SignMessage(msg []byte) (sig []byte, err error) {
	// Before signing, we need to hash the message
	// The hash is what we actually sign
	msgHash := sha512.New()
	if _, err = msgHash.Write(msg); err != nil {
		glog.Exitf("Message hashing failed %v", err)
	}
	msgHashSum := msgHash.Sum(nil)

	// Get the required key for signing
	key, err := getPrivateKey()
	if err != nil {
		glog.Exitf("Private key fetch failed %v", err)
	}

	signature, err := rsa.SignPSS(rand.Reader, key, crypto.SHA512, msgHashSum, nil)
	if err != nil {
		glog.Exitf("Signing failed for firmware statement: %v", err)
		return nil, err
	}
	return signature, nil
}

//VerifySignature is used to verify the incoming message
func VerifySignature(msg []byte, signature []byte) (verified bool, err error) {
	// Get the required key for signing
	key, err := getPublicKey()

	// Before verify, we need to hash the message
	// The hash is what we actually verify
	msgHash := sha512.New()
	if _, err = msgHash.Write(msg); err != nil {
		glog.Exitf("Message hashing failed %v", err)
	}
	msgHashSum := msgHash.Sum(nil)

	if err = rsa.VerifyPSS(key, crypto.SHA512, msgHashSum, signature, nil); err != nil {
		glog.Warningf("Failed to verify signature: %q", err)
		return false, err
	}
	// If we don't get any error from the `VerifyPSS` method, implies our
	// signature is valid
	return true, nil
}
