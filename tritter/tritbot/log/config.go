package log

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"

	tc "github.com/google/trillian/client"
	"github.com/google/trillian/merkle/rfc6962"
)

// TreeVerifier returns a verifier configured for the log.
func TreeVerifier() (*tc.LogVerifier, error) {
	pk, err := getTrillianPK()
	if err != nil {
		return nil, fmt.Errorf("failed to load Trillian public key: %v", err)
	}

	return tc.NewLogVerifier(rfc6962.DefaultHasher, *pk, crypto.SHA256), nil
}

func getTrillianPK() (*crypto.PublicKey, error) {
	// go run github.com/google/trillian/cmd/get_tree_public_key --admin_server=localhost:50054 --log_id=3564243390614880449
	trillianPubKey := []byte(`
-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEJR3RjSqkBUuhbp4674WuqAO0WIln
aMOM3IHek85J0IngSoNE6Vsw+lZ8YPtbGZz1k9L6yA8R3Yru26JKsGwOVQ==
-----END PUBLIC KEY-----`)

	block, _ := pem.Decode(trillianPubKey)
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, errors.New("failed to decode PEM block containing public key")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	pk := pub.(crypto.PublicKey)
	return &pk, nil
}
