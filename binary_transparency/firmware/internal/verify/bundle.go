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
// limitations under the License.

// Package verify holds helpers for validating the correctness of various
// artifacts and proofs used in the system.
package verify

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/crypto"
)

// BundleForUpdate checks that the manifest, checkpoint, and proofs in a bundle
// are all self-consistent, and that the provided firmware image hash matches
// the one in the bundle. It also checks consistency proof between update log point
// and device log point (for non zero device tree size).
func BundleForUpdate(b api.ProofBundle, fwHash []byte, dc api.LogCheckpoint, cProof [][]byte) error {
	fwMeta, err := verifyBundle(b)
	if err != nil {
		return err
	}

	if got, want := fwHash, fwMeta.FirmwareImageSHA512; !bytes.Equal(got, want) {
		return fmt.Errorf("firmware update image hash does not match metadata (0x%x != 0x%x)", got, want)
	}
	// Verify the consistency proof between device and bundle checkpoint
	if dc.TreeSize > 0 {
		lv := NewLogVerifier()
		if err := lv.VerifyConsistencyProof(int64(b.Checkpoint.TreeSize), int64(dc.TreeSize), b.Checkpoint.RootHash, dc.RootHash, cProof); err != nil {
			return fmt.Errorf("failed verification of consistency proof %w", err)

		}
	}
	return nil
}

// BundleForBoot checks that the manifest, checkpoint, and proofs in a bundle
// are all self-consistent, and that the provided firmware measurement matches
// the one expected by the bundle.
func BundleForBoot(b api.ProofBundle, measurement []byte) error {
	fwMeta, err := verifyBundle(b)
	if err != nil {
		return err
	}

	if got, want := measurement, fwMeta.ExpectedFirmwareMeasurement; !bytes.Equal(got, want) {
		return fmt.Errorf("firmware measurement does not match metadata (0x%x != 0x%x)", got, want)
	}
	return nil
}

// verifyBundle verifies the self-consistency of a proof bundle.
func verifyBundle(b api.ProofBundle) (api.FirmwareMetadata, error) {
	var fwStatement api.FirmwareStatement
	if err := json.Unmarshal(b.ManifestStatement, &fwStatement); err != nil {
		return api.FirmwareMetadata{}, fmt.Errorf("failed to unmarshal FirmwareStatement: %w", err)
	}
	// Verify the statement signature:
	if err := crypto.VerifySignature(fwStatement.Metadata, fwStatement.Signature); err != nil {
		return api.FirmwareMetadata{}, fmt.Errorf("failed to verify signature on FirmwareStatement: %w", err)
	}

	var fwMeta api.FirmwareMetadata
	if err := json.Unmarshal(fwStatement.Metadata, &fwMeta); err != nil {
		return api.FirmwareMetadata{}, fmt.Errorf("failed to unmarshal Metadata: %w", err)
	}

	lh := HashLeaf(b.ManifestStatement)
	lv := NewLogVerifier()

	if err := lv.VerifyInclusionProof(int64(b.InclusionProof.LeafIndex), int64(b.Checkpoint.TreeSize), b.InclusionProof.Proof, b.Checkpoint.RootHash, lh); err != nil {
		return api.FirmwareMetadata{}, fmt.Errorf("invalid inclusion proof in bundle: %w", err)
	}
	return fwMeta, nil
}
