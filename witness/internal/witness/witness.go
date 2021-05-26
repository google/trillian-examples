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
	Parsed *log.Checkpoint
	Raw    []byte
}

type ChkptStorage interface {
	// GetLatest returns the latest checkpoint for a given log.
	GetLatest(logPK string) (*Chkpt, error)

	// SetCheckpoint adds a checkpoint to the storage for a given log.
	SetCheckpoint(logPK string, c Chkpt) error
}

type Witness struct {
	db     ChkptStorage
	Signer note.Signer
	Hasher hashers.LogHasher
	LogV   logverifier.LogVerifier
	SigVs  []note.Verifier
}

// NewWitness creates a new witness, which initially has no logs to follow.
func NewWitness(h hashers.LogHasher, db ChkptStorage, sk string) (*Witness, error) {
	ns, err := note.NewSigner(sk)
	if err != nil {
		return nil, err
	}
	return &Witness{
		db:     db,
		Signer: ns,
		Hasher: h,
		LogV:   logverifier.New(h),
		SigVs:  []note.Verifier{},
	}, nil
}

// parse verifies the checkpoint under the given list of possible public keys.
// It returns the parsed checkpoint and the list of public keys under which the
// signature(s) verified.
func (w *Witness) parse(chkptRaw []byte, verifiers []note.Verifier) (*log.Checkpoint, []string, error) {
	n, err := note.Open(chkptRaw, note.VerifierList(verifiers...))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to verify checkpoint: %w", err)
	}
	chkpt := &log.Checkpoint{}
	_, err = chkpt.Unmarshal([]byte(n.Text))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal new checkpoint: %w", err)
	}
	var keys []string
	for _, sig := range n.Sigs {
		keys = append(keys, sig.Name)
	}
	return chkpt, keys, nil
}

// GetLatest gets the latest checkpoint for a given log, which should be
// consistent with all other checkpoints for the same log.  It also signs it
// under the witness' key.
func (w *Witness) GetLatest(logPK string) ([]byte, error) {
	chkpt, err := w.db.GetLatest(logPK)
	if err != nil {
		return nil, err
	}
	// Add the witness' signature to the checkpoint.
	// TODO find a way so we don't have to verify the signatures again.
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

// Update updates latest checkpoint if `to` matches the current version (`from`).
func (w *Witness) Update(fromRaw, toRaw []byte, proof log.Proof) error {
	// Check the signatures on both of the raw checkpoints and parse them
	// into the log.Checkpoint format.
	from, keysFrom, err := w.parse(fromRaw, w.SigVs)
	if err != nil {
		return err
	}
	to, keysTo, err := w.parse(toRaw, w.SigVs)
	if err != nil {
		return err
	}
	// Make sure both verified with respect to the same public key.
	if len(keysFrom) > 1 || len(keysTo) > 1 {
		return fmt.Errorf("Bad signature verification")
	}
	if keysFrom[0] != keysTo[0] {
		return fmt.Errorf("Checkpoints verified under different public keys")
	}
	// Get the latest one because we don't want consistency proofs with
	// respect to older checkpoints.
	logPK := keysFrom[0]
	latest, err := w.db.GetLatest(logPK)
	if err != nil {
		return err
	}
	// Don't want `from` to be too small.
	if from.Size < latest.Parsed.Size {
		return ErrStale{
			Smaller: from,
			Latest:  latest.Parsed,
		}
	}
	// Don't want `from` to be too big.
	if from.Size > latest.Parsed.Size {
		return fmt.Errorf("Checkpoints are disconnected from current view")
	}
	if from.Size > 0 {
		if to.Size > from.Size {
			if err := w.LogV.VerifyConsistencyProof(int64(from.Size), int64(to.Size), from.Hash, to.Hash, proof); err != nil {
				// Complain if the checkpoints aren't consistent.
				return ErrInconsistency{
					Smaller: fromRaw,
					Larger:  toRaw,
					Proof:   proof,
					Wrapped: err,
				}
			}
			// If the consistency proof is good we store `to`.
			return w.db.SetCheckpoint(logPK, Chkpt{Parsed: to, Raw: toRaw})
		} else {
			// Complain if `to` is bigger than `from`.
			return fmt.Errorf("Cannot prove consistency backwards")
		}
	}
	// Complain if the size of `from` is too small.
	return fmt.Errorf("Invalid checkpoint given as source")
}

// Registers a new log with the TOFU checkpoint.
func (w *Witness) Register(logPK string, chkptRaw []byte) error {
	nv, err := note.NewVerifier(logPK)
	if err != nil {
		return err
	}
	// Verify the signature on the checkpoint with respect to only this log.
	chkpt, _, err := w.parse(chkptRaw, []note.Verifier{nv})
	if err != nil {
		return err
	}
	// Add a signature verifier for this public key.
	w.SigVs = append(w.SigVs, nv)
	return w.db.SetCheckpoint(logPK, Chkpt{Parsed: chkpt, Raw: chkptRaw})
}
