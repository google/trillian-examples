// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package note

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/mod/sumdb/note"
)

const (
	algEd25519              = 1
	algEd25519WithTimestamp = 3
)

// NewSignerWithTimestamps constructs a new Signer that produces timestamped
// signatures from a standard encoded signer key.
//
// The returned Signer has a different key hash from a non-timestamped one,
// meaning it will differ from the key hash in the input encoding.
func NewSignerWithTimestamps(skey string) (note.Signer, error) {
	priv1, skey, _ := strings.Cut(skey, "+")
	priv2, skey, _ := strings.Cut(skey, "+")
	name, skey, _ := strings.Cut(skey, "+")
	hash16, key64, _ := strings.Cut(skey, "+")
	key, err := base64.StdEncoding.DecodeString(key64)
	if priv1 != "PRIVATE" || priv2 != "KEY" || len(hash16) != 8 || err != nil || !isValidName(name) || len(key) == 0 {
		return nil, errSignerID
	}

	s := &signer{name: name}

	alg, key := key[0], key[1:]
	switch alg {
	default:
		return nil, errSignerAlg

	case algEd25519:
		if len(key) != ed25519.SeedSize {
			return nil, errSignerID
		}
		key := ed25519.NewKeyFromSeed(key)
		pubkey := append([]byte{algEd25519WithTimestamp}, key.Public().(ed25519.PublicKey)...)
		s.hash = keyHashEd25519(name, pubkey)
		s.sign = func(msg []byte) ([]byte, error) {
			t := uint64(time.Now().Unix())
			// The signed message starts with a 0x00 to provide domain
			// separation from ordinary note signatures (notes can't contain
			// control characters), TLS (messages start with 64 ASCII spaces),
			// and X.509 (messages start with 0x30).
			//
			// Ideally we'd use Ed25519ctx instead, but its availability is limited.
			m := make([]byte, 1, 1+8+len(msg))
			m = binary.LittleEndian.AppendUint64(m, t)
			m = append(m, msg...)
			// The signature itself is timestamp || signature.
			sig := make([]byte, 0, 8+ed25519.SignatureSize)
			sig = binary.LittleEndian.AppendUint64(sig, t)
			sig = append(sig, ed25519.Sign(key, m)...)
			return sig, nil
		}
	}

	return s, nil
}

var (
	errSignerID   = errors.New("malformed verifier id")
	errSignerAlg  = errors.New("unknown verifier algorithm")
	errSignerHash = errors.New("invalid verifier hash")
)

// signer is a trivial Signer implementation.
type signer struct {
	name string
	hash uint32
	sign func([]byte) ([]byte, error)
}

func (s *signer) Name() string                    { return s.name }
func (s *signer) KeyHash() uint32                 { return s.hash }
func (s *signer) Sign(msg []byte) ([]byte, error) { return s.sign(msg) }

// isValidName reports whether name is valid.
// It must be non-empty and not have any Unicode spaces or pluses.
func isValidName(name string) bool {
	return name != "" && utf8.ValidString(name) && strings.IndexFunc(name, unicode.IsSpace) < 0 && !strings.Contains(name, "+")
}

func keyHashEd25519(name string, key []byte) uint32 {
	h := sha256.New()
	h.Write([]byte(name))
	h.Write([]byte("\n"))
	h.Write(key)
	sum := h.Sum(nil)
	return binary.BigEndian.Uint32(sum)
}
