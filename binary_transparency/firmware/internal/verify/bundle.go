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

// ConsistencyProofFunc is a function which returns a consistency proof between two tree sizes.
type ConsistencyProofFunc func(from, to uint64) ([][]byte, error)

// BundleForUpdate checks that the manifest, checkpoint, and proofs in a bundle
// are all self-consistent, and that the provided firmware image hash matches
// the one in the bundle. It also checks consistency proof between update log point
// and device log point (for non zero device tree size).
func BundleForUpdate(bundleRaw, fwHash []byte, dc api.LogCheckpoint, cpFunc ConsistencyProofFunc) error {
	proofBundle, fwMeta, err := verifyBundle(bundleRaw)
	if err != nil {
		return err
	}

	if got, want := fwHash, fwMeta.FirmwareImageSHA512; !bytes.Equal(got, want) {
		return fmt.Errorf("firmware update image hash does not match metadata (0x%x != 0x%x)", got, want)
	}

	cProof, err := cpFunc(dc.TreeSize, proofBundle.Checkpoint.TreeSize)
	if err != nil {
		return fmt.Errorf("cpFunc failed: %q", err)
	}

	// Verify the consistency proof between device and bundle checkpoint
	if dc.TreeSize > 0 {
		lv := NewLogVerifier()
		if err := lv.VerifyConsistencyProof(int64(dc.TreeSize), int64(proofBundle.Checkpoint.TreeSize), dc.RootHash, proofBundle.Checkpoint.RootHash, cProof); err != nil {
			return fmt.Errorf("failed verification of consistency proof %w", err)
		}
	}
	return nil
}

// BundleForBoot checks that the manifest, checkpoint, and proofs in a bundle
// are all self-consistent, and that the provided firmware measurement matches
// the one expected by the bundle.
func BundleForBoot(bundleRaw, measurement []byte) error {
	_, fwMeta, err := verifyBundle(bundleRaw)
	if err != nil {
		return err
	}

	if got, want := measurement, fwMeta.ExpectedFirmwareMeasurement; !bytes.Equal(got, want) {
		return fmt.Errorf("firmware measurement does not match metadata (0x%x != 0x%x)", got, want)
	}
	return nil
}

// verifyBundle parses a proof bundle and verifies its self-consistency.
func verifyBundle(bundleRaw []byte) (api.ProofBundle, api.FirmwareMetadata, error) {
	var pb api.ProofBundle
	if err := json.Unmarshal(bundleRaw, &pb); err != nil {
		return api.ProofBundle{}, api.FirmwareMetadata{}, fmt.Errorf("failed to parse proof bundle: %w", err)
	}

	// TODO(al): check Checkpoint signature

	var fwStatement api.FirmwareStatement
	if err := json.Unmarshal(pb.ManifestStatement, &fwStatement); err != nil {
		return api.ProofBundle{}, api.FirmwareMetadata{}, fmt.Errorf("failed to unmarshal FirmwareStatement: %w", err)
	}
	// Verify the statement signature:
	if err := crypto.VerifySignature(fwStatement.Metadata, fwStatement.Signature); err != nil {
		return api.ProofBundle{}, api.FirmwareMetadata{}, fmt.Errorf("failed to verify signature on FirmwareStatement: %w", err)
	}

	lh := HashLeaf(pb.ManifestStatement)
	lv := NewLogVerifier()
	if err := lv.VerifyInclusionProof(int64(pb.InclusionProof.LeafIndex), int64(pb.Checkpoint.TreeSize), pb.InclusionProof.Proof, pb.Checkpoint.RootHash, lh); err != nil {
		return api.ProofBundle{}, api.FirmwareMetadata{}, fmt.Errorf("invalid inclusion proof in bundle: %w", err)
	}

	var fwMeta api.FirmwareMetadata
	if err := json.Unmarshal(fwStatement.Metadata, &fwMeta); err != nil {
		return api.ProofBundle{}, api.FirmwareMetadata{}, fmt.Errorf("failed to unmarshal Metadata: %w", err)
	}

	return pb, fwMeta, nil
}
