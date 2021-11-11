// https://github.com/f-secure-foundry/armory-boot
//
// Copyright (c) F-Secure Corporation
// https://foundry.f-secure.com
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

//go:build armory
// +build armory

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

func verifySignature(bin []byte, sig []byte, pubKey string) (err error) {
	s, err := DecodeSignature(string(sig))

	if err != nil {
		return fmt.Errorf("invalid signature, %v", err)
	}

	pub, err := NewPublicKey(pubKey)

	if err != nil {
		return fmt.Errorf("invalid public key, %v", err)
	}

	valid, err := pub.Verify(bin, s)

	if err != nil {
		return fmt.Errorf("invalid signature, %v", err)
	}

	if !valid {
		return errors.New("invalid signature")
	}

	return
}

func verifyHash(bin []byte, s string) bool {
	h := sha256.New()
	h.Write(bin)

	hash, err := hex.DecodeString(s)

	if err != nil {
		return false
	}

	return bytes.Equal(h.Sum(nil), hash)
}
