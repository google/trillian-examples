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
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
)

var (
	// Publisher makes statements containing the firmware metadata.
	Publisher = Claimant{
		priv: TestVendorRSAPriv,
		pub:  TestVendorRSAPub,
	}

	// AnnotatorMalware makes annotation statements about malware in firmware.
	AnnotatorMalware = Claimant{
		priv: TestAnnotationPriv,
		pub:  TestAnnotationPub,
	}
)

// Claimant is someone that makes statements, and can sign and verify them.
type Claimant struct {
	// Note that outside of a demo the private key should never be used like this!
	priv, pub string
}

func (c *Claimant) getPrivateKey() (*rsa.PrivateKey, error) {
	mKey := strings.NewReader(c.priv)
	priv, err := io.ReadAll(mKey)
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

func (c *Claimant) getPublicKey() (*rsa.PublicKey, error) {
	mKey := strings.NewReader(c.pub)
	pub, err := io.ReadAll(mKey)
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

// SignMessage is used to sign the Statement
func (c *Claimant) SignMessage(stype api.StatementType, msg []byte) ([]byte, error) {
	if len(msg) > 64*1024*1024 {
		return nil, errors.New("msg too large")
	}
	bs := make([]byte, len(msg)+1)
	bs[0] = byte(stype)
	copy(bs[1:], msg)

	// Before signing, we need to hash the message
	// The hash is what we actually sign
	h := sha512.Sum512(bs)

	// Get the required key for signing
	key, err := c.getPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("private key fetch failed %v", err)
	}
	// use PSS over PKCS#1 v1.5 for enhanced security
	signature, err := rsa.SignPSS(rand.Reader, key, crypto.SHA512, h[:], nil)
	if err != nil {
		return nil, fmt.Errorf("failed to sign statement %v", err)
	}
	return signature, nil
}

// VerifySignature is used to verify the incoming message
func (c *Claimant) VerifySignature(stype api.StatementType, stmt []byte, signature []byte) error {
	// Get the required key for signing
	key, err := c.getPublicKey()
	if err != nil {
		return fmt.Errorf("public key fetch failed %v", err)
	}
	bs := make([]byte, len(stmt)+1)
	bs[0] = byte(stype)
	copy(bs[1:], stmt)

	// Before verify, we need to hash the message
	// The hash is what we actually verify
	h := sha512.Sum512(bs)

	if err = rsa.VerifyPSS(key, crypto.SHA512, h[:], signature, nil); err != nil {
		return fmt.Errorf("failed to verify signature %v", err)
	}
	// If we don't get any error from the `VerifyPSS` method, implies our
	// signature is valid
	return nil
}

// ClaimantForType returns the relevant Claimant for the given Statement type.
func ClaimantForType(t api.StatementType) (*Claimant, error) {
	switch t {
	case api.FirmwareMetadataType:
		return &Publisher, nil
	case api.MalwareStatementType:
		return &AnnotatorMalware, nil
	default:
		return nil, fmt.Errorf("Unknown Claimant type %v", t)
	}
}
