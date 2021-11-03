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
	"encoding/binary"

	"golang.org/x/mod/sumdb/note"
)

// ecdsaVerifier is a note-compatible verifier for ECDSA signatures.
type ecdsaVerifier struct {
	name    string
	keyHash uint32
	v       func(msg, sig []byte) bool
}

// Name returns the name associated with the key this verifier is based on.
func (e *ecdsaVerifier) Name() string {
	return e.name
}

// KeyHash returns a truncated hash of the key this verifier is based on.
func (e *ecdsaVerifier) KeyHash() uint32 {
	return e.keyHash
}

// Verify checks that the provided sig is valid over msg for the key this verifier is based on.
func (e *ecdsaVerifier) Verify(msg, sig []byte) bool {
	return e.v(msg, sig)
}

// NewSigstoreVerifier creates a new note verifier for checking ECDSA signatures over SHA256 digests.
// This implementation is compatible with the signature scheme used by the Sigstore Rékor Log.
//
// Note that in contrast with the note Ed25519 signer, the Sigstore log produces signatures whose
// keyhash hints do not include the key name in the preimage.
func NewSigstoreVerifier(name string, pubK *ecdsa.PublicKey) (note.Verifier, error) {
	// We need the public key in SubjectPublicKeyInfo format for creating the keyhash below.
	spki, err := x509.MarshalPKIXPublicKey(pubK)
	if err != nil {
		return nil, err
	}
	kh := sha256.Sum256(spki)

	return &ecdsaVerifier{
		name: name,
		v: func(msg, sig []byte) bool {
			dgst := sha256.Sum256(msg)
			return ecdsa.VerifyASN1(pubK, dgst[:], sig)
		},
		keyHash: binary.BigEndian.Uint32(kh[:]),
	}, nil
}
