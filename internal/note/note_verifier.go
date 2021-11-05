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

// package note provides note-compatible signature verifiers.
package note

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"

	sdb_note "golang.org/x/mod/sumdb/note"
)

const (
	// Note represents a key type that the Go SumDB note will
	// know about.
	Note = ""

	// SigstoreECDSA is an ECDSA signature over SHA256.
	// The current implementation of this has a different KeyHash input
	// to the SumDB Note's Ed25519 verifier.
	SigstoreECDSA = "sigstore_ecdsa"
)

func NewVerifier(keyType, key string) (sdb_note.Verifier, error) {
	switch keyType {
	case SigstoreECDSA:
		return NewSigstoreECDSAVerifier(key)
	case Note:
		return sdb_note.NewVerifier(key)
	default:
		return nil, fmt.Errorf("unknown key type %q", keyType)
	}
}

// verifier is a note-compatible verifier.
type verifier struct {
	name    string
	keyHash uint32
	v       func(msg, sig []byte) bool
}

// Name returns the name associated with the key this verifier is based on.
func (v *verifier) Name() string {
	return v.name
}

// KeyHash returns a truncated hash of the key this verifier is based on.
func (v *verifier) KeyHash() uint32 {
	return v.keyHash
}

// Verify checks that the provided sig is valid over msg for the key this verifier is based on.
func (v *verifier) Verify(msg, sig []byte) bool {
	return v.v(msg, sig)
}

// NewSigstoreECDSAVerifier creates a new note verifier for checking ECDSA signatures over SHA256 digests.
// This implementation is compatible with the signature scheme used by the Sigstore Rékor Log.
//
// The key is expected to be provided as a string in the following form:
//   <key name> " " <Base64 encoded SubjectPublicKeyInfo DER>
// e.g.:
//   "rekor.sigstore.dev MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE2G2Y+2tabdTV5BcGiBIx0a9fAFwrkBbmLSGtks4L3qX6yYY0zufBnhC8Ur/iy55GhWP/9A/bY2LhC30M9+RYtw=="
//
// Note that in contrast with the note Ed25519 signer, the Sigstore log produces signatures whose
// keyhash hints do not include the key name in the preimage.
func NewSigstoreECDSAVerifier(key string) (sdb_note.Verifier, error) {
	parts := strings.SplitN(key, " ", 2)
	if got, want := len(parts), 2; got != want {
		return nil, fmt.Errorf("key has %d parts, expected %d: %q", got, want, key)
	}
	der, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("key has invalid base64 %q: %v", parts[1], err)
	}
	k, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse public key: %v", err)
	}
	ecdsaKey, ok := k.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("key is a %T, expected an ECDSA key", k)
	}

	kh := sha256.Sum256(der)

	return &verifier{
		name: parts[0],
		v: func(msg, sig []byte) bool {
			dgst := sha256.Sum256(msg)
			return ecdsa.VerifyASN1(ecdsaKey, dgst[:], sig)
		},
		keyHash: binary.BigEndian.Uint32(kh[:]),
	}, nil
}
