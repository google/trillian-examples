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

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/crypto"
	"github.com/transparency-dev/merkle/proof"
	"github.com/transparency-dev/merkle/rfc6962"
	"golang.org/x/mod/sumdb/note"
)

// ConsistencyProofFunc is a function which returns a consistency proof between two tree sizes.
type ConsistencyProofFunc func(from, to uint64) ([][]byte, error)

// BundleForUpdate checks that the manifest, checkpoint, and proofs in a raw bundle
// are all self-consistent, and that the provided firmware image hash matches
// the one in the bundle. It also checks consistency proof between update log point
// and device log point (for non zero device tree size). Upon successful verification
// returns a proof bundle
func BundleForUpdate(bundleRaw, fwHash []byte, dc api.LogCheckpoint, cpFunc ConsistencyProofFunc, logSigVerifier note.Verifier) (api.ProofBundle, api.FirmwareMetadata, error) {
	proofBundle, fwMeta, err := verifyBundle(bundleRaw, logSigVerifier)
	if err != nil {
		return proofBundle, fwMeta, err
	}

	if got, want := fwHash, fwMeta.FirmwareImageSHA512; !bytes.Equal(got, want) {
		return proofBundle, fwMeta, fmt.Errorf("firmware update image hash does not match metadata (0x%x != 0x%x)", got, want)
	}

	pc, err := api.ParseCheckpoint(proofBundle.Checkpoint, logSigVerifier)
	if err != nil {
		return proofBundle, fwMeta, fmt.Errorf("failed to open the device checkpoint: %w", err)
	}

	cProof, err := cpFunc(dc.Size, pc.Size)
	if err != nil {
		return proofBundle, fwMeta, fmt.Errorf("cpFunc failed: %q", err)
	}

	// Verify the consistency proof between device and bundle checkpoint
	if dc.Size > 0 {
		h := rfc6962.DefaultHasher
		if err := proof.VerifyConsistency(h, dc.Size, pc.Size, cProof, dc.Hash, pc.Hash); err != nil {
			return proofBundle, fwMeta, fmt.Errorf("failed verification of consistency proof %w", err)
		}
	}
	return proofBundle, fwMeta, nil
}

// BundleConsistency verifies the log checkpoint in the bundle is consistent against a given checkpoint (e.g. one fetched from a witness).
func BundleConsistency(pb api.ProofBundle, rc api.LogCheckpoint, cpFunc ConsistencyProofFunc, logSigVerifier note.Verifier) error {

	glog.V(1).Infof("Remote TreeSize=%d, Inclusion Index=%d \n", rc.Size, pb.InclusionProof.LeafIndex)
	if rc.Size < pb.InclusionProof.LeafIndex {
		return fmt.Errorf("remote verification failed wcp treesize(%d)<device cp index(%d)", rc.Size, pb.InclusionProof.LeafIndex)
	}

	bundleCP, err := api.ParseCheckpoint(pb.Checkpoint, logSigVerifier)
	if err != nil {
		return fmt.Errorf("failed to open the proof bundle checkpoint: %w", err)
	}
	fromCP, toCP := rc, *bundleCP
	// swap the remote checkpoint(fromCP) with published checkpoint (toCP) if it is ahead of published checkpoint
	if rc.Size > bundleCP.Size {
		fromCP, toCP = toCP, fromCP
	}
	cProof, err := cpFunc(fromCP.Size, toCP.Size)
	if err != nil {
		return fmt.Errorf("cpFunc failed: %q", err)
	}
	h := rfc6962.DefaultHasher
	if err := proof.VerifyConsistency(h, fromCP.Size, toCP.Size, cProof, fromCP.Hash, toCP.Hash); err != nil {
		return fmt.Errorf("failed consistency proof between remote and client checkpoint %w", err)
	}
	return nil
}

// BundleForBoot checks that the manifest, checkpoint, and proofs in a bundle
// are all self-consistent, and that the provided firmware measurement matches
// the one expected by the bundle.
func BundleForBoot(bundleRaw, measurement []byte, logSigVerifier note.Verifier) error {
	_, fwMeta, err := verifyBundle(bundleRaw, logSigVerifier)
	if err != nil {
		return err
	}

	if got, want := measurement, fwMeta.ExpectedFirmwareMeasurement; !bytes.Equal(got, want) {
		return fmt.Errorf("firmware measurement does not match metadata (0x%x != 0x%x)", got, want)
	}
	return nil
}

// verifyBundle parses a proof bundle and verifies its self-consistency.
func verifyBundle(bundleRaw []byte, logSigVerifier note.Verifier) (api.ProofBundle, api.FirmwareMetadata, error) {
	var pb api.ProofBundle
	if err := json.Unmarshal(bundleRaw, &pb); err != nil {
		return api.ProofBundle{}, api.FirmwareMetadata{}, fmt.Errorf("failed to parse proof bundle: %w", err)
	}

	bundleCP, err := api.ParseCheckpoint(pb.Checkpoint, logSigVerifier)
	if err != nil {
		return api.ProofBundle{}, api.FirmwareMetadata{}, fmt.Errorf("failed to open the proof bundle checkpoint: %w", err)
	}

	var fwStatement api.SignedStatement
	if err := json.Unmarshal(pb.ManifestStatement, &fwStatement); err != nil {
		return api.ProofBundle{}, api.FirmwareMetadata{}, fmt.Errorf("failed to unmarshal SignedStatement: %w", err)
	}
	// Verify the statement signature:
	if err := crypto.Publisher.VerifySignature(fwStatement.Type, fwStatement.Statement, fwStatement.Signature); err != nil {
		return api.ProofBundle{}, api.FirmwareMetadata{}, fmt.Errorf("failed to verify signature on SignedStatement: %w", err)
	}
	if fwStatement.Type != api.FirmwareMetadataType {
		return api.ProofBundle{}, api.FirmwareMetadata{}, fmt.Errorf("expected statement type %q, but got %q", api.FirmwareMetadataType, fwStatement.Type)
	}

	h := rfc6962.DefaultHasher
	lh := h.HashLeaf(pb.ManifestStatement)
	if err := proof.VerifyInclusion(h, pb.InclusionProof.LeafIndex, bundleCP.Size, lh, pb.InclusionProof.Proof, bundleCP.Hash); err != nil {
		return api.ProofBundle{}, api.FirmwareMetadata{}, fmt.Errorf("invalid inclusion proof in bundle: %w", err)
	}

	var fwMeta api.FirmwareMetadata
	if err := json.Unmarshal(fwStatement.Statement, &fwMeta); err != nil {
		return api.ProofBundle{}, api.FirmwareMetadata{}, fmt.Errorf("failed to unmarshal Metadata: %w", err)
	}

	return pb, fwMeta, nil
}
