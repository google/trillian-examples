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

const (
	// These come from the the current SigStore Rekór key, which is an ECDSA key:
	sigStoreKeyMaterial = "AjBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABNhtmPtrWm3U1eQXBogSMdGvXwBcK5AW5i0hrZLOC96l+smGNM7nwZ4QvFK/4sueRoVj//QP22Ni4Qt9DPfkWLc="
	sigStoreKeyHash     = "c0d23d6a"
	sigStoreKey         = "rekor.sigstore.dev" + "+" + sigStoreKeyHash + "+" + sigStoreKeyMaterial

	// These come from the the current Pixel6 log key, which is an ECDSA key.
	// KeyMaterial converted from PEM contents here: https://go.dev/play/p/xKGbOGW_JHZ
	pixelKeyMaterial = "AjBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABN+4x0Jk1yTwvLFI9A4NDdGZcX0aiWVdWM5XJVy0M4VWD3AvyW5Q6Hs9A0mcDkpUoYgn+KKPNzFC0H3nN3q6JQ8="
	pixelKeyHash     = "91c16e30"
	pixelKey         = "pixel6_transparency_log" + "+" + pixelKeyHash + "+" + pixelKeyMaterial
)

func TestNewVerifier(t *testing.T) {
	for _, test := range []struct {
		name    string
		keyType string
		key     string
		wantErr bool
	}{
		{
			name: "note works",
			key:  "PeterNeumann+c74f20a3+ARpc2QcUPDhMQegwxbzhKqiBfsVkmqq/LDE4izWy10TW",
		}, {
			name:    "note mismatch",
			key:     sigStoreKey,
			wantErr: true,
		}, {
			name:    "ECDSA works",
			keyType: ECDSA,
			key:     sigStoreKey,
		}, {
			name:    "ECDSA mismatch",
			keyType: ECDSA,
			key:     "PeterNeumann+c74f20a3+ARpc2QcUPDhMQegwxbzhKqiBfsVkmqq/LDE4izWy10TW",
			wantErr: true,
		}, {
			name:    "unknown type fails",
			keyType: "bananas",
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewVerifier(test.keyType, test.key)
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
			name: "sigStore works",
			pubK: sigStoreKey,
		}, {
			name: "pixel works",
			pubK: pixelKey,
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
			name: "sigstore works",
			pubK: sigStoreKey,
			note: []byte("Rekor\n798034\nf+7CoKgXKE/tNys9TTXcr/ad6U/K3xvznmzew9y6SP0=\n\n— rekor.sigstore.dev wNI9ajBEAiARInWIWyCdyG27CO6LPnPekyw20qO0YJfoaPaowGp/XgIgc+qEHS3+GKVClgqq20uDLet7MCoTURUCRdxwWBHHufk=\n"),
		}, {
			name: "pixel works",
			pubK: pixelKey,
			note: []byte("DEFAULT\n10\nbsWRucJU5xJPHb5eBdOm6+DM+VelCZBuvtI3sHERJ9Y=\n\n— pixel6_transparency_log kcFuMDBFAiEAhqMAP8P6qf6QxtUJhzMhbN+MbZ9dwfUHzGQJmffJHtoCIGD0cNe47dHWBoPwYdgBCepB06/+g5O1FmYjXl06owL4\n"),
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
