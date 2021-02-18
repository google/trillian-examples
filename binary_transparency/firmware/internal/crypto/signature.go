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

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
)

var (
	// Publisher makes statements containing the firmware metadata.
	Publisher = Claimant{
		priv: RSApri,
		pub:  RSApub,
	}

	// AnnotatorMalware makes annotation statements about malware in firmware.
	AnnotatorMalware = Claimant{
		priv: `-----BEGIN RSA PRIVATE KEY-----
MIIEogIBAAKCAQEAoGKwBzNMxdPS1Uo+BAykf2C9nuLpLkXBSpINYOiGcJeBpV04
MUw7BrW0ynvwwQL1h+yVWfiRL3xMDQmMkr/EjPEiW4VXji/lVMDClIi4mlMdygiG
hM1Na5PpCAg8+khpFOnSlpC95EdGoU3c+iezTJH0nHH6KUfT6cMZMLNdjI5MtES3
aZET4/iQRkmqv+FXP4M5QmQb1PE7iEGZ6J3IUQKJrpqilz0RZM535Tz89cchD1Mh
cIwi3MO45dbZP1/0lDDYinK/VnklYlKuZjXTC29TWFzStF34YPDA1dFaGZug2/oC
jdMLyQBEZJsEYEwF6jhdp4p7zHOSQ5Xc68xbDQIDAQABAoIBADUsgu/gMjPkZqIQ
Wz88cc1JZZSn5mdQ+SSgB495iBkMIg+ROHAftfIjjC0VqlxTftPxvBJ4Nqpnq08n
O1PsAF46FAoDy2N4va+7uMdGDO4dYGL7MJ4W8vQXtcrT8GOKXkxwuUDx/AMTHnec
OQc24lsgiNjVcPr+tWNrK47Z6MoQXT7CaZpQMaZkYxj8T3g63Anfdjoichv4jByS
/TQzGElqea1IxzUpvGXMf3n6IV4GxlVdDX0rFXLM9vp7k5Av8vBx4YCnTi48u0m0
JbdAzfgeOla8GkWzzgYSkAY0CMhiSNB6eUkbO9qxHH0o0JZKuU8kP1bci/LBhcCL
MyeGSAECgYEAzTrYxne3hUeVxAmXABtbgWtZbmlfE3FKXf+Jy4UOaLa/KZeHAaIK
e32KjYTITfNlNZ+g6nYAy1hI7kJvutnznjY3Nfv9gtRNmK96ej+1SabMD68raDGY
7/YR5mYr7KcI9Am5HFKTzDKBHUry0OB3v5JOPMrr9Le/LZ6pxQzKOT0CgYEAyA/b
0bCylSGX2LW63IuEdyPEv5zIL/7cTsUNdtWbSofBpG3DdT9Fm2AvfnipJRxNIcX+
t5zAqPrWSx23ovhr7GsdPawjn8oNpwO5GyHmpiFiMtMnykEAXtQNuJbPGcpKFWnH
Ydp8pSBVyK0C5vlJWINNHYa0vTEzKW1MwbBVphECgYAu8PzQOGXDmGILGt5s6dT+
Px2PgY57lfgak+5inKZ1EQecbco1d2jKYiakw/BE1B0cLMzTk/YOjLzxskR4Co4M
a/4o3OBZYlH1UH3FJHlExV/7XmehR2bhy/jAKDJ3yKTlnKu4bLLdi9e4aYIsgIsj
SEWY5hkeOkECID5YkdpXSQKBgAuG3mN2itOM2/LghaOvZjJ3HR7tKZuaU5c2Q1BV
fl0M9VtD978JpjkNka73xMcemlMX1VU+8trJmQ865xm8tnsosMac5HCQc7jrvf6S
NXfc9It5HxHILP1JuoCoL8aMoTgaoCJDNGtPMaIeVcx5EIDJD+hjmoZMD2aTpZiD
UGwBAoGAIDPx1CWMWqwLIF66BfbxnYC7iPKWc0oTUKFx45yxjgwegzj5n9VCmJw0
nSEBOasrfQKsWg0gbtgoxxg6awY12czAWRukp5zyoTT+PSGi32gHepCOan4MqJ5w
QeDeXVCJOme4xiBEhnC95flKCsfN9yBMwFy2N7qj0T1DbyKZtSc=
-----END RSA PRIVATE KEY-----`,
		pub: `-----BEGIN RSA PUBLIC KEY-----
MIIBCgKCAQEAoGKwBzNMxdPS1Uo+BAykf2C9nuLpLkXBSpINYOiGcJeBpV04MUw7
BrW0ynvwwQL1h+yVWfiRL3xMDQmMkr/EjPEiW4VXji/lVMDClIi4mlMdygiGhM1N
a5PpCAg8+khpFOnSlpC95EdGoU3c+iezTJH0nHH6KUfT6cMZMLNdjI5MtES3aZET
4/iQRkmqv+FXP4M5QmQb1PE7iEGZ6J3IUQKJrpqilz0RZM535Tz89cchD1MhcIwi
3MO45dbZP1/0lDDYinK/VnklYlKuZjXTC29TWFzStF34YPDA1dFaGZug2/oCjdML
yQBEZJsEYEwF6jhdp4p7zHOSQ5Xc68xbDQIDAQAB
-----END RSA PUBLIC KEY-----`,
	}
)

// Claimant is someone that makes statements, and can sign and verify them.
type Claimant struct {
	// Note that outside of a demo the private key should never be used like this!
	priv, pub string
}

func (c *Claimant) getPrivateKey() (*rsa.PrivateKey, error) {
	mKey := strings.NewReader(c.priv)
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

func (c *Claimant) getPublicKey() (*rsa.PublicKey, error) {
	mKey := strings.NewReader(c.pub)
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

// SignMessage is used to sign the Statement
func (c *Claimant) SignMessage(stype api.StatementType, msg []byte) ([]byte, error) {
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
