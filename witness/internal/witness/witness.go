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
// limitations under the License.

package witness

import (
	"fmt"

	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/merkle/logverifier"
	"golang.org/x/mod/sumdb/note"
)

type Chkpt struct {
	Size uint64
	Raw  []byte
}

type ChkptStorage interface {
	// GetLatest returns the latest checkpoint for a given log.
	GetLatest(logPK string) (*Chkpt, error)

	// SetCheckpoint adds a checkpoint to the storage for a given log.
	SetCheckpoint(logPK string, c Chkpt) error
}

type WitnessOpts struct {
	Ecosystem string
	Hasher    hashers.LogHasher
	Storage   ChkptStorage
	Signer    note.Signer
	KnownLogs []note.Verifier
}

type Witness struct {
	db     ChkptStorage
	Signer note.Signer
	Hasher hashers.LogHasher
	LogV   logverifier.LogVerifier
	SigVs  []note.Verifier
}

// NewWitness creates a new witness, which initially has no logs to follow.
func NewWitness(wo WitnessOpts) *Witness {
	return &Witness{
		db:     wo.Storage,
		Signer: wo.Signer,
		Hasher: wo.Hasher,
		LogV:   logverifier.New(wo.Hasher),
		SigVs:  wo.KnownLogs,
	}
}

// parse verifies the checkpoint. It returns the parsed checkpoint and the list
// of public keys under which the signature(s) verified.
func (w *Witness) parse(chkptRaw []byte) (*log.Checkpoint, []string, error) {
	n, err := note.Open(chkptRaw, note.VerifierList(w.SigVs...))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to verify checkpoint: %w", err)
	}
	chkpt := &log.Checkpoint{}
	_, err = chkpt.Unmarshal([]byte(n.Text))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal new checkpoint: %w", err)
	}
	keys := make([]string, len(n.Sigs))
	for i, sig := range n.Sigs {
		keys[i] = sig.Name
	}
	return chkpt, keys, nil
}

// GetLatest gets the latest checkpoint for a given log, which will be
// consistent with all other checkpoints for the same log.  It also signs it
// under the witness' key.
func (w *Witness) GetLatest(logPK string) ([]byte, error) {
	chkpt, err := w.db.GetLatest(logPK)
	if err != nil {
		return nil, err
	}
	// Add the witness' signature to the checkpoint.
	n, err := note.Open(chkpt.Raw, note.VerifierList(w.SigVs...))
	if err != nil {
		return nil, fmt.Errorf("failed to verify checkpoint: %w", err)
	}
	cosigned, err := note.Sign(n, w.Signer)
	if err != nil {
		return nil, err
	}
	return cosigned, nil
}

// Update updates latest checkpoint if chkptRaw is consistent with the current
// latest one for this log.
func (w *Witness) Update(chkptRaw []byte, proof log.Proof) error {
	// Check the signatures on the raw checkpoint and parse it
	// into the log.Checkpoint format.
	chkpt, keys, err := w.parse(chkptRaw)
	if err != nil {
		return err
	}
	// Make sure it verified with respect to only one public key.
	if len(keys) > 1 {
		return fmt.Errorf("Bad signature verification")
	}
	// Get the latest one for the log, as identified by the key, because we
	// don't want consistency proofs with respect to older checkpoints.
	logPK := keys[0]
	latest, err := w.db.GetLatest(logPK)
	if err != nil {
		return err
	}
	p, _, _ := w.parse(latest.Raw)
	if chkpt.Size > p.Size {
		if err := w.LogV.VerifyConsistencyProof(int64(p.Size), int64(chkpt.Size), p.Hash, chkpt.Hash, proof); err != nil {
			// Complain if the checkpoints aren't consistent.
			return ErrInconsistency{
				Smaller: latest.Raw,
				Larger:  chkptRaw,
				Proof:   proof,
				Wrapped: err,
			}
		}
		// If the consistency proof is good we store chkpt.
		return w.db.SetCheckpoint(logPK, Chkpt{Size: chkpt.Size, Raw: chkptRaw})
	}
	// Complain if latest is bigger than chkpt.
	return fmt.Errorf("Cannot prove consistency backwards")
}
