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
// limitations under the License.package bundle_test

package verify_test

import (
	"bytes"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/crypto"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/verify"
	"github.com/transparency-dev/merkle/proof"
	"golang.org/x/mod/sumdb/note"
)

const (
	goldenProofBundle   = `{"ManifestStatement":"eyJUeXBlIjoxMDIsIlN0YXRlbWVudCI6ImV5SkVaWFpwWTJWSlJDSTZJbVIxYlcxNUlpd2lSbWx5YlhkaGNtVlNaWFpwYzJsdmJpSTZNU3dpUm1seWJYZGhjbVZKYldGblpWTklRVFV4TWlJNkltZ3ZTblpLTURVeE1GZE5Ua05hZG1wWFQwTXdVMUZxTDFKUFJHVXpLMGh6Uld0dE5HMUhUbnBWVEhSd1lXVlVhblkyU2xrcmQzSTBlVk51Vm5aNVJqVXdZa055TVhSd2NYZERiMEZvWm5CeFFscHNNbUpSUFQwaUxDSkZlSEJsWTNSbFpFWnBjbTEzWVhKbFRXVmhjM1Z5WlcxbGJuUWlPaUkyZEZWWGVYbHJlbmRtYjI5dVIzVm5ibVl4WkV3clkyZGtObUpGVjFWb2IyeEJUbUZFVERoS1dVdFhkR1J0VUdORlpWbDJaMDgyYmsxeUwwbE1aMWRRWTFWWloyZDJRVUZ5Y25SaFUwSnRZVzQxU0RSTFp6MDlJaXdpUW5WcGJHUlVhVzFsYzNSaGJYQWlPaUl5TURJd0xURXdMVEV3VkRFMU9qTXdPakl3TGpFd1dpSjkiLCJTaWduYXR1cmUiOiJUeUxVdFpCdHJHbyt3anRoSjI2Rk8wVE5QUHpTTDZhU0c1V0ZTanRCcjZLZ2x4a0RjR3dmZUxTTEpjbklmUnhZMnVJZHZLL09tMStXMndLNEkxSFRUYTdIUFZlSHo2MmF0V09hZm9TL1ZGc01OdEx1RkplaU5WNE5uY2Y0bllYMFBDdnN0MHRpYm5TVFRzNnEwMUZ1cEhaMnFwc2lyY2hFVXgwLzFjOFFOM2hGZVArSXcwVWxPNTVvZUhlWGtlRGRwL2w5SCsvZjYxYndFMmpHZVl1cFcvbld2bmN2NFgrS00weXgrYW1oVi9od0lCMDQ2aitNQzVndkd4LzJ2TkkySk5JeTBOQk13YWRIK1VONnp1MzRIZzM5NkY4MkxJU2NTOXU2MENBY3hzRlozR3d5NGpoR1JXR1lwSnhXdEJ6Zk5hZ1IvaVdmUzRJY2tJVmZ5Z2ZQUXc9PSJ9","Checkpoint":"RmlybXdhcmUgVHJhbnNwYXJlbmN5IExvZwo1Clg2VEY4QWNkSUh2OVp0UWwrU1NhZVZOYy9aNVJjNDJweDFpRlJLVGNDdHc9CjE2MDc0NTA3MzgxMTE1MDYwODgKCuKAlCBmdF9wZXJzb25hbGl0eSA2S0pDdlg1NTYrSGxNSnozMlhnQVhBYit1ODVOUEpSTkt2eHJ5enU1WTVibGkxWTh0YURsNDNjS2lmeVdsT1pYQWVnYndCaDUyUlNWc3ZJSUovTXY1K2Z0WHdjPQo=","InclusionProof":{"Value":null,"LeafIndex":4,"Proof":["KFh4IVeIwbsvbWyz2QHVCXXyjWTRDqusRa0ZEjS2fls="]}}`
	goldenFirmwareImage = `Firmware image`
	// goldenFirmwareHashB64 is a base64 encoded string for ExpectedMeasurement field inside ManifestStatement.
	// For the dummy device, this is SHA512("dummy"||img), where img is the base64 decoded bytes from
	// goldenUpdate.FirmwareImage
	goldenFirmwareHashB64 = "6tUWyykzwfoonGugnf1dL+cgd6bEWUholANaDL8JYKWtdmPcEeYvgO6nMr/ILgWPcUYggvAArrtaSBman5H4Kg=="
)

func mustGetLogSigVerifier(t *testing.T) note.Verifier {
	t.Helper()
	v, err := note.NewVerifier(crypto.TestFTPersonalityPub)
	if err != nil {
		t.Fatalf("failed to create verifier: %q", err)
	}
	return v
}

// This test is useful for creating the Checkpoint field of the goldenProofBundle above.
func TestGenerateGoldenCheckpoint(t *testing.T) {
	cp := "RmlybXdhcmUgVHJhbnNwYXJlbmN5IExvZwo1Clg2VEY4QWNkSUh2OVp0UWwrU1NhZVZOYy9aNVJjNDJweDFpRlJLVGNDdHc9CjE2MDc0NTA3MzgxMTE1MDYwODgKCuKAlCBmdF9wZXJzb25hbGl0eSA2S0pDdlg1NTYrSGxNSnozMlhnQVhBYit1ODVOUEpSTkt2eHJ5enU1WTVibGkxWTh0YURsNDNjS2lmeVdsT1pYQWVnYndCaDUyUlNWc3ZJSUovTXY1K2Z0WHdjPQo="
	nb, _ := base64.StdEncoding.DecodeString(cp)
	s, err := note.NewSigner(crypto.TestFTPersonalityPriv)
	if err != nil {
		t.Fatalf("failed to create signer: %q", err)
	}
	n, err := note.Sign(&note.Note{Text: string(nb)}, s)
	if err != nil {
		t.Fatalf("failed to sign note: %q", err)
	}
	t.Log(string(n))
	t.Log(base64.StdEncoding.EncodeToString(n))
}

func TestBundleForUpdate(t *testing.T) {
	var dc api.LogCheckpoint
	getProof := func(from, to uint64) ([][]byte, error) { return [][]byte{}, nil }

	for _, test := range []struct {
		desc    string
		img     []byte
		wantErr bool
	}{
		{
			desc: "all good",
			img:  []byte(goldenFirmwareImage),
		}, {
			desc:    "bad image hash",
			img:     []byte("this is wrong"),
			wantErr: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			imgHash := sha512.Sum512(test.img)
			_, _, err := verify.BundleForUpdate([]byte(goldenProofBundle), imgHash[:], dc, getProof, mustGetLogSigVerifier(t))
			if (err != nil) != test.wantErr {
				var lve proof.RootMismatchError
				if errors.As(err, &lve) {
					// Printing this out allows `goldenProofBundle` to be updated if needed
					t.Errorf("calculated root %s", base64.StdEncoding.EncodeToString(lve.CalculatedRoot))
				}
				t.Fatalf("want err %v, got %q", test.wantErr, err)
			}
		})
	}
}

func b64Decode(t *testing.T, b64 string) []byte {
	t.Helper()
	st, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("b64 decoding failed: %v", err)
	}
	return st
}

func TestBundleForBoot(t *testing.T) {
	for _, test := range []struct {
		desc        string
		measurement []byte
		wantErr     bool
	}{
		{
			desc:        "all good",
			measurement: []byte(b64Decode(t, goldenFirmwareHashB64)),
		}, {
			desc:        "bad image hash",
			measurement: []byte("this is wrong"),
			wantErr:     true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			err := verify.BundleForBoot([]byte(goldenProofBundle), test.measurement, mustGetLogSigVerifier(t))
			if (err != nil) != test.wantErr {
				t.Fatalf("want err %v, got %q", test.wantErr, err)
			}
		})
	}
}

// This test is really showing how the constants above are generated, and allows
// them to be regenerated should any of the underlying formats change.
func TestGoldenBundleGeneration(t *testing.T) {
	h := sha512.Sum512([]byte(goldenFirmwareImage))
	meta := api.FirmwareMetadata{
		DeviceID:                    "dummy",
		FirmwareRevision:            1,
		FirmwareImageSHA512:         h[:],
		ExpectedFirmwareMeasurement: b64Decode(t, goldenFirmwareHashB64),
		BuildTimestamp:              "2020-10-10T15:30:20.10Z",
	}

	mbs, _ := json.Marshal(meta)
	sig, err := crypto.Publisher.SignMessage(api.FirmwareMetadataType, mbs)
	if err != nil {
		t.Error(err)
	}
	ss := api.SignedStatement{
		Type:      api.FirmwareMetadataType,
		Statement: mbs,
		Signature: sig,
	}
	var gpb api.ProofBundle
	if err := json.Unmarshal([]byte(goldenProofBundle), &gpb); err != nil {
		t.Error(err)
	}
	var gss api.SignedStatement
	if err := json.Unmarshal(gpb.ManifestStatement, &gss); err != nil {
		t.Error(err)
	}
	// Signature can't go into this check because they are non-deterministic.
	if gss.Type != ss.Type || !bytes.Equal(gss.Statement, ss.Statement) {
		gbs, _ := json.Marshal(gss)
		sbs, _ := json.Marshal(ss)
		// If this fails then the golden values in the test may need updating with `sbs`
		t.Errorf("Golden != computed: %s, %s", gbs, sbs)
	}

}
