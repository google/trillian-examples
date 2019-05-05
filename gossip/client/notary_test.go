// Copyright 2019 Google Inc. All Rights Reserved.
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

package client_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/google/certificate-transparency-go/jsonclient"
	"github.com/google/certificate-transparency-go/tls"
	"github.com/google/trillian-examples/gossip/api"
	"github.com/google/trillian-examples/gossip/client"
	"golang.org/x/crypto/ed25519"
)

// Taken from golang.org/x/exp/sumdb/internal/note.
const algEd25519 = byte(1)

var (
	testPrivKeyRaw = "AYEKFALVFGyNhPJEMzD1QIDr+Y7hfZx09iUvxdXHKDFz"
	testPubKeyRaw  = "ARpc2QcUPDhMQegwxbzhKqiBfsVkmqq/LDE4izWy10TW"
	testMessage    = "If you think cryptography is the answer to your problem,\n" +
		"then you don't know what your problem is.\n"
	testSignature     = "x08go/ZJkuBS9UG/SffcvIAQxVBtiFupLLr8pAcElZInNIuGUgYN1FFYC2pZSNXgKvqfqdngotpRZb6KE6RyyBwJnAM="
	testSignatureLine = "— PeterNeumann " + testSignature
	testNote          = testMessage + "\n" + testSignatureLine + "\n"

	// From https://github.com/golang/go/blob/3cf1d770809453fed8754cf3ddf4028cdd7716fe/src/cmd/go/internal/modfetch/key.go
	sumGolangOrgPubKeyRaw = "Ac4zctda0e5eza+HJyk9SxEdh+s3Ux18htTTAD8OuAn8"
	// From sum.golang.org:
	sumGolangOrgLatest    = "go.sum database tree\n13485\n/ZWur4obeiqK+ASdnPK+/Rmz6LPQMUeSmHh0+UkL54Y=\n"
	sumGolangOrgLatestSig = "Az3grnQReYTvpCrRxT+w50vixbqBlB/aPS8bL8f8KW4HHP+UjLSXOJgCh4f54n8hO6lrESglzyRXMXnhaWzaeY3zqw8="
)

func TestKeyData(t *testing.T) {
	checkKeysSignature(t, testPrivKeyRaw, testPubKeyRaw, testMessage, testSignature)
	checkKeysSignature(t, "", sumGolangOrgPubKeyRaw, sumGolangOrgLatest, sumGolangOrgLatestSig)
}

func checkKeysSignature(t *testing.T, b64PrivKey, b64PubKey, msg, b64Sig string) {
	t.Helper()

	pubKeyData, err := base64.StdEncoding.DecodeString(b64PubKey)
	if err != nil {
		t.Fatalf("failed to base64-decode public key: %v", err)
	}
	switch got := len(pubKeyData); got {
	case ed25519.PublicKeySize + 1:
		// Expect a single byte algorithm identifier prefix.
		if got, want := pubKeyData[0], algEd25519; got != want {
			t.Fatalf("pubKeyData[0]=%02x, want=%02x", got, want)
		}
		pubKeyData = pubKeyData[1:]
	case ed25519.PublicKeySize:
		// No prefix.
	default:
		t.Fatalf("len(%s=>seed=%x)=%d, want %d +? 1", b64PubKey, pubKeyData, got, ed25519.PublicKeySize)
	}
	pubKey := ed25519.PublicKey(pubKeyData)

	if len(b64PrivKey) > 0 {
		// If a private key was provided, check it too.
		privKeySeed, err := base64.StdEncoding.DecodeString(b64PrivKey)
		if err != nil {
			t.Fatalf("failed to base64-decode private key: %v", err)
		}

		switch got := len(privKeySeed); got {
		case ed25519.SeedSize + 1:
			// Expect a single byte algorithm identifier prefix.
			if got, want := privKeySeed[0], algEd25519; got != want {
				t.Fatalf("privData[0]=%02x, want=%02x", got, want)
			}
			privKeySeed = privKeySeed[1:]

		case ed25519.SeedSize:
			// No prefix.
		default:
			t.Fatalf("len(%s=>seed=%x)=%d, want %d + 1?", b64PrivKey, privKeySeed, got, ed25519.SeedSize)
		}

		privKey := ed25519.NewKeyFromSeed(privKeySeed)
		regenPubKey, ok := privKey.Public().(ed25519.PublicKey)
		if !ok {
			t.Fatal("failed to convert privKey.Public() to ed25519.PublicKey")
		}
		// Check the private key recreates the provided public key.
		if got, want := regenPubKey, pubKey; !reflect.DeepEqual(got, want) {
			t.Errorf("NewKeyFromSeed(%x).Public()=%x, want %x", privKeySeed, got, want)
		}
	}

	sigData, err := base64.StdEncoding.DecodeString(b64Sig)
	if err != nil {
		t.Fatalf("failed to base64-decode signature: %v", err)
	}
	// First 4 bytes of signature form a big-endian u32 hash of the key.
	signature := sigData[4:]

	if got, want := len(signature), ed25519.SignatureSize; got != want {
		t.Fatalf("len(%s=>sig=%x)=%d, want %d", b64Sig, sigData, got, want)
	}

	if !ed25519.Verify(pubKey, []byte(msg), signature) {
		t.Errorf("Verify(pubKey=%s, msg=%q, sig=%s)=false, want true", base64.StdEncoding.EncodeToString(pubKey), msg, b64Sig)
	}
}

func TestAddSignedNote(t *testing.T) {
	ctx := context.Background()
	entry := api.TimestampedEntry{
		SourceID:        []byte("PeterNeumann"),
		BlobData:        []byte(testMessage),
		SourceSignature: []byte{0xDD},
		HubTimestamp:    0x0000000012345678,
	}
	data, err := tls.Marshal(entry)
	if err != nil {
		t.Fatalf("Failed to create test data: %v", err)
	}
	signer, pubKeyDER := generateKeys(t)

	tests := []struct {
		desc    string
		source  string
		note    string
		body    string
		wantErr string
	}{
		{
			desc:   "valid",
			source: "PeterNeumann",
			note:   testNote,
			body:   fmt.Sprintf(`{"timestamped_entry":"%s", "hub_signature":"%s"}`, signData(t, signer, data)...),
		},
		{
			desc:    "no-separator",
			source:  "PeterNeumann",
			note:    testMessage,
			wantErr: "no separator",
		},
		{
			desc:    "no-signatures",
			source:  "PeterNeumann",
			note:    testMessage + "\n\n",
			wantErr: "no signatures",
		},
		{
			desc:   "valid-after-junk",
			source: "PeterNeumann",
			note:   testMessage + "\n" + "junk\n" + testSignatureLine + "\n",
			body:   fmt.Sprintf(`{"timestamped_entry":"%s", "hub_signature":"%s"}`, signData(t, signer, data)...),
		},
		{
			desc:   "valid-after-junk-2",
			source: "PeterNeumann",
			note:   testMessage + "\n" + "— junkyjunk\n" + testSignatureLine + "\n",
			body:   fmt.Sprintf(`{"timestamped_entry":"%s", "hub_signature":"%s"}`, signData(t, signer, data)...),
		},
		{
			desc:   "valid-after-junk-3",
			source: "PeterNeumann",
			note:   testMessage + "\n" + "— junky junk\n" + testSignatureLine + "\n",
			body:   fmt.Sprintf(`{"timestamped_entry":"%s", "hub_signature":"%s"}`, signData(t, signer, data)...),
		},
		{
			desc:    "wrong-source",
			source:  "NeumannMarcus",
			note:    testNote,
			wantErr: "no signature for source",
		},
		{
			desc:    "empty-signature",
			source:  "PeterNeumann",
			note:    testMessage + "\n" + "— PeterNeumann " + "\n",
			wantErr: "empty signature",
		},
		{
			desc:    "invalid-base64",
			source:  "PeterNeumann",
			note:    testMessage + "\n" + "— PeterNeumann x08go/ZJkuBS9UG/SffcvIAQxVBtiFupLLr8pAcElZInNIuGUgYN1FFYC2pZSNXgKvqfqdngotpRZb6KE6RyyBnAM=" + "\n",
			wantErr: "failed to parse",
		},
		{
			desc:    "short-signature",
			source:  "PeterNeumann",
			note:    testMessage + "\n" + "— PeterNeumann AQID" + "\n",
			wantErr: "too short",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ts := serveAt(t, api.AddSignedBlobPath, http.StatusOK, test.body)
			defer ts.Close()
			cl, err := client.New(ts.URL, &http.Client{}, jsonclient.Options{PublicKeyDER: pubKeyDER})
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			got, gotErr := cl.AddSignedNote(ctx, test.source, []byte(test.note))
			if gotErr != nil {
				if len(test.wantErr) == 0 {
					t.Errorf("AddSignedNote()=nil,%v; want _, nil", gotErr)
				} else if !strings.Contains(gotErr.Error(), test.wantErr) {
					t.Errorf("AddSignedNote()=nil,%v; want _, err containing %q", gotErr, test.wantErr)
				}
				return
			}
			if len(test.wantErr) != 0 {
				t.Errorf("AddSignedNote()=%v,nil; want _, err containing %q", got, test.wantErr)
			}
			if got == nil {
				t.Errorf("AddSignedNote()=nil; want non-nil")
			}
		})
	}
}
