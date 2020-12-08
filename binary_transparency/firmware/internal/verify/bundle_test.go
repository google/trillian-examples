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
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/verify"
)

const (
	goldenUpdate        = `{"FirmwareImage":"RmlybXdhcmUgaW1hZ2U=","ProofBundle":{"ManifestStatement":"eyJNZXRhZGF0YSI6ImV5SkVaWFpwWTJWSlJDSTZJbVIxYlcxNUlpd2lSbWx5YlhkaGNtVlNaWFpwYzJsdmJpSTZNU3dpUm1seWJYZGhjbVZKYldGblpWTklRVFV4TWlJNkltZ3ZTblpLTURVeE1GZE5Ua05hZG1wWFQwTXdVMUZxTDFKUFJHVXpLMGh6Uld0dE5HMUhUbnBWVEhSd1lXVlVhblkyU2xrcmQzSTBlVk51Vm5aNVJqVXdZa055TVhSd2NYZERiMEZvWm5CeFFscHNNbUpSUFQwaUxDSkZlSEJsWTNSbFpFWnBjbTEzWVhKbFRXVmhjM1Z5WlcxbGJuUWlPaUkyZEZWWGVYbHJlbmRtYjI5dVIzVm5ibVl4WkV3clkyZGtObUpGVjFWb2IyeEJUbUZFVERoS1dVdFhkR1J0VUdORlpWbDJaMDgyYmsxeUwwbE1aMWRRWTFWWloyZDJRVUZ5Y25SaFUwSnRZVzQxU0RSTFp6MDlJaXdpUW5WcGJHUlVhVzFsYzNSaGJYQWlPaUl5TURJd0xURXdMVEV3VkRFMU9qTXdPakl3TGpFd1dpSjkiLCJTaWduYXR1cmUiOiJFaXBNMXRMdjF4cnJMSHZEdC80VDFHUG9KV3hBYlExMmhiMkZTOWQ1cDhsbjBKeWJJZFBieVBPWTVYMXozbVBUV0VnMnp1VTB1aWs0VmQwNW84dmM1cGRPZEFTSHlCeDA5RXBhT0NjTWVxSW9SRm90N3lvVUVDdUxkZHBCNEw4aWlEZ3Vibnk1Tk8zaTkzTjNFcnBUclN0b1ZqWjd1ZnZRd082SWg4aWpQZTVTY0o5TG1zQjBMRkZKeUIvQVNnYXcyeE9NWDVnMjlxSzR5UWNBak11WlE3b25ITG95Z09pK2pWUy92akJ0SEVxcXQ1RVU3dU9NdVJVSitqOFYva25yWUJya2hMNEVqWW9SZFNKTnZ6azVpMDRrdGNWLzJQb1NBR2RqSi9rejMrUG1idStXUjRRMVZMcng2bzBaVFNRMi94dXR2K1d2K0lmQXFOdDB0QldoWnc9PSJ9","Checkpoint":{"TreeSize":5,"RootHash":"4E7J8K809jeqeg1oiTIz+5zfMItqZqUBFR0jySa3H/M=","TimestampNanos":1607450738111506088},"InclusionProof":{"Value":null,"LeafIndex":4,"Proof":["KFh4IVeIwbsvbWyz2QHVCXXyjWTRDqusRa0ZEjS2fls="]}}}`
	goldenFirmwareImage = `Firmware image`
)

func TestBundleForUpdate(t *testing.T) {
	var up api.UpdatePackage
	var dc api.LogCheckpoint
	var proof [][]byte
	if err := json.Unmarshal([]byte(goldenUpdate), &up); err != nil {
		t.Fatalf(err.Error())
	}

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
			err := verify.BundleForUpdate(up.ProofBundle, imgHash[:], dc, proof)
			if (err != nil) != test.wantErr {
				t.Fatalf("want err %T, got %q", test.wantErr, err)
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
	var up api.UpdatePackage
	if err := json.Unmarshal([]byte(goldenUpdate), &up); err != nil {
		t.Fatalf(err.Error())
	}

	b64 := "6tUWyykzwfoonGugnf1dL+cgd6bEWUholANaDL8JYKWtdmPcEeYvgO6nMr/ILgWPcUYggvAArrtaSBman5H4Kg=="
	for _, test := range []struct {
		desc        string
		measurement []byte
		wantErr     bool
	}{
		{
			desc:        "all good",
			measurement: []byte(b64Decode(t, b64)),
		}, {
			desc:        "bad image hash",
			measurement: []byte("this is wrong"),
			wantErr:     true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			err := verify.BundleForBoot(up.ProofBundle, test.measurement)
			if (err != nil) != test.wantErr {
				t.Fatalf("want err %T, got %q", test.wantErr, err)
			}
		})
	}
}
