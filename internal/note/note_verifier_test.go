package note

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"testing"

	"golang.org/x/mod/sumdb/note"
)

// SigStoreKeyB64 is the current rékor dev log public key.
const sigStoreKeyB64 = "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE2G2Y+2tabdTV5BcGiBIx0a9fAFwrkBbmLSGtks4L3qX6yYY0zufBnhC8Ur/iy55GhWP/9A/bY2LhC30M9+RYtw=="

func TestSigstoreVerifier(t *testing.T) {
	for _, test := range []struct {
		name     string
		pubKName string
		pubK     *ecdsa.PublicKey
		note     []byte
		wantErr  bool
	}{
		{
			name:     "works",
			pubKName: "rekor.sigstore.dev",
			pubK:     mustCreateKey(t, sigStoreKeyB64),
			note:     []byte("Rekor\n798034\nf+7CoKgXKE/tNys9TTXcr/ad6U/K3xvznmzew9y6SP0=\n\n— rekor.sigstore.dev wNI9ajBEAiARInWIWyCdyG27CO6LPnPekyw20qO0YJfoaPaowGp/XgIgc+qEHS3+GKVClgqq20uDLet7MCoTURUCRdxwWBHHufk=\n"),
		}, {
			name:     "invalid name",
			pubKName: "bananas.sigstore.dev",
			pubK:     mustCreateKey(t, sigStoreKeyB64),
			note:     []byte("Rekor\n798034\nf+7CoKgXKE/tNys9TTXcr/ad6U/K3xvznmzew9y6SP0=\n\n— rekor.sigstore.dev wNI9ajBEAiARInWIWyCdyG27CO6LPnPekyw20qO0YJfoaPaowGp/XgIgc+qEHS3+GKVClgqq20uDLet7MCoTURUCRdxwWBHHufk=\n"),
			wantErr:  true,
		}, {
			name:     "invalid signature",
			pubKName: "rekor.sigstore.dev",
			pubK:     mustCreateKey(t, sigStoreKeyB64),
			note:     []byte("Rekor\n798034\nf+7CoKgXKE/tNys9TTXcr/ad6U/K3xvznmzew9y6SP0=\n\n— rekor.sigstore.dev THIS/IS/PROBABLY/NOT/A/VALID/SIGNATURE/ANy/MOREowGp/XgIgc+qEHS3+GKVClgqq20uDLet7MCoTURUCRdxwWBHHufk=\n"),
			wantErr:  true,
		},
	} {
		v, err := NewSigstoreVerifier(test.pubKName, test.pubK)
		if err != nil {
			t.Fatalf("Failed to create new ECDSA verifier: %v", err)
		}
		_, err = note.Open(test.note, note.VerifierList(v))
		if gotErr := err != nil; gotErr != test.wantErr {
			t.Fatalf("Got err %v, but want error %v", err, test.wantErr)
		}
	}
}

func mustCreateKey(t *testing.T, b64 string) *ecdsa.PublicKey {
	der, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("Failed to base64 decode key: %v", err)
	}
	k, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		t.Fatalf("Failed to parse public key: %v", err)
	}
	e, ok := k.(*ecdsa.PublicKey)
	if !ok {
		t.Fatalf("Expected ecdsa.PublicKey, but got %T", k)
	}
	return e
}
