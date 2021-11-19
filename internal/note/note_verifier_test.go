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
// limitations under the License.package note

package note

import (
	"testing"

	"golang.org/x/mod/sumdb/note"
)

// These come from the the current SigStore Rekór key, which is an ECDSA key:
const (
	sigStoreKeyMaterial = "AjBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABNhtmPtrWm3U1eQXBogSMdGvXwBcK5AW5i0hrZLOC96l+smGNM7nwZ4QvFK/4sueRoVj//QP22Ni4Qt9DPfkWLc="
	sigStoreKeyHash     = "c0d23d6a"
	sigStoreKey         = "rekor.sigstore.dev" + "+" + sigStoreKeyHash + "+" + sigStoreKeyMaterial
)

func TestNewVerifier(t *testing.T) {
	for _, test := range []struct {
		name    string
		kType   string
		k       string
		wantErr bool
	}{
		{
			name: "note works",
			k:    "PeterNeumann+c74f20a3+ARpc2QcUPDhMQegwxbzhKqiBfsVkmqq/LDE4izWy10TW",
		}, {
			name:    "note mismatch",
			k:       sigStoreKey,
			wantErr: true,
		}, {
			name:  "ECDSA works",
			kType: ECDSA,
			k:     sigStoreKey,
		}, {
			name:    "ECDSA mismatch",
			kType:   ECDSA,
			k:       "PeterNeumann+c74f20a3+ARpc2QcUPDhMQegwxbzhKqiBfsVkmqq/LDE4izWy10TW",
			wantErr: true,
		}, {
			name:    "unknown type fails",
			kType:   "bananas",
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewVerifier(test.kType, test.k)
			if gotErr := err != nil; gotErr != test.wantErr {
				t.Fatalf("NewVerifier: %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestNewECDSAVerifier(t *testing.T) {
	for _, test := range []struct {
		name    string
		pubK    string
		wantErr bool
	}{
		{
			name: "works",
			pubK: sigStoreKey,
		}, {
			name:    "wrong number of parts",
			pubK:    "bananas.sigstore.dev+12344556",
			wantErr: true,
		}, {
			name:    "invalid base64",
			pubK:    "rekor.sigstore.dev+12345678+THIS_IS_NOT_BASE64!",
			wantErr: true,
		}, {
			name:    "invalid algo",
			pubK:    "rekor.sigstore.dev+12345678+AwEB",
			wantErr: true,
		}, {
			name:    "invalid keyhash",
			pubK:    "rekor.sigstore.dev+NOT_A_NUMBER+" + sigStoreKeyMaterial,
			wantErr: true,
		}, {
			name:    "incorrect keyhash",
			pubK:    "rekor.sigstore.dev" + "+" + "00000000" + "+" + sigStoreKeyMaterial,
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewECDSAVerifier(test.pubK)
			if gotErr := err != nil; gotErr != test.wantErr {
				t.Fatalf("Failed to create new ECDSA verifier from %q: %v", test.pubK, err)
			}
		})
	}
}
func TestECDSAVerifier(t *testing.T) {
	for _, test := range []struct {
		name    string
		pubK    string
		note    []byte
		wantErr bool
	}{
		{
			name: "works",
			pubK: sigStoreKey,
			note: []byte("Rekor\n798034\nf+7CoKgXKE/tNys9TTXcr/ad6U/K3xvznmzew9y6SP0=\n\n— rekor.sigstore.dev wNI9ajBEAiARInWIWyCdyG27CO6LPnPekyw20qO0YJfoaPaowGp/XgIgc+qEHS3+GKVClgqq20uDLet7MCoTURUCRdxwWBHHufk=\n"),
		}, {
			name:    "invalid name",
			pubK:    "bananas.sigstore.dev" + "+" + sigStoreKeyHash + "+" + sigStoreKeyMaterial,
			note:    []byte("Rekor\n798034\nf+7CoKgXKE/tNys9TTXcr/ad6U/K3xvznmzew9y6SP0=\n\n— rekor.sigstore.dev wNI9ajBEAiARInWIWyCdyG27CO6LPnPekyw20qO0YJfoaPaowGp/XgIgc+qEHS3+GKVClgqq20uDLet7MCoTURUCRdxwWBHHufk=\n"),
			wantErr: true,
		}, {
			name:    "invalid signature",
			pubK:    sigStoreKey,
			note:    []byte("Rekor\n798034\nf+7CoKgXKE/tNys9TTXcr/ad6U/K3xvznmzew9y6SP0=\n\n— rekor.sigstore.dev THIS/IS/PROBABLY/NOT/A/VALID/SIGNATURE/ANy/MOREowGp/XgIgc+qEHS3+GKVClgqq20uDLet7MCoTURUCRdxwWBHHufk=\n"),
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			v, err := NewECDSAVerifier(test.pubK)
			if err != nil {
				t.Fatalf("Failed to create new ECDSA verifier from %q: %v", test.pubK, err)
			}
			_, err = note.Open(test.note, note.VerifierList(v))
			if gotErr := err != nil; gotErr != test.wantErr {
				t.Fatalf("Got err %v, but want error %v", err, test.wantErr)
			}
		})
	}
}
