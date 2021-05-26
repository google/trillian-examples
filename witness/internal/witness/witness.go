package witness

import (
	"fmt"

	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/merkle/logverifier"
	"golang.org/x/mod/sumdb/note"
)

type ChkptStorage interface {
	// GetLatest returns the latest checkpoint for a given log.
	GetLatest(logId string) (*log.Checkpoint, error)

	// SetCheckpoint adds a checkpoint to the storage for a given log.
	SetCheckpoint(logId string, c *log.Checkpoint) error
}

type Witness struct {
	db       ChkptStorage
	Hasher   hashers.LogHasher
	LogV	 logverifier.LogVerifier
	SigV	 note.Verifier
}

// NewWitness creates a new witness, which initially has no logs to follow.
func NewWitness(h hashers.LogHasher, db ChkptStorage, nv note.Verifier) *Witness {
	return &Witness{
		db:       db,
		Hasher:   h,
		LogV: logverifier.New(h),
		SigV: nv,
	}
}

// GetLatest gets the latest checkpoint for a given log, which should be
// consistent with all other checkpoints for the same log.  It also signs it
// under the witness' key.
func (w *Witness) GetLatest(logId string) (*log.Checkpoint, error) {
	return w.db.GetLatest(logId)
}

func (w *Witness) verifyParse(rawChkpt []byte) (*log.Checkpoint, error) {
	n, err := note.Open(rawChkpt, note.VerifierList(w.SigV))
	if err != nil {
		return nil, fmt.Errorf("failed to verify checkpoint: %w", err)
	}
	chkpt := &log.Checkpoint{}
	_, err = chkpt.Unmarshal([]byte(n.Text))
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal new checkpoint: %w", err)
	}
	return chkpt, nil
}

// Update updates latest checkpoint if `to` matches the current version (`from`).
func (w *Witness) Update(logId string, from, to []byte, proof log.Proof) error {
	// Check the signatures on both of the raw checkpoints and parse them
	// into the log.Checkpoint format.
	fromChkpt, err := w.verifyParse(from)
	if err != nil {
		return err
	}
	toChkpt, err := w.verifyParse(to)
	if err != nil {
		return err
	}
	// Get the latest one because we don't want consistency proofs with
	// respect to older checkpoints.
	latest, err := w.db.GetLatest(logId)
	if err != nil {
		return err
	}
	if fromChkpt.Size < latest.Size {
		return ErrStale{
			Smaller: fromChkpt,
			Latest:  latest,
		}
	}
	if fromChkpt.Size > 0 {
		if toChkpt.Size > fromChkpt.Size {
			if err := w.LogV.VerifyConsistencyProof(int64(fromChkpt.Size), int64(toChkpt.Size), fromChkpt.Hash, toChkpt.Hash, proof); err != nil {
				return ErrInconsistency{
					Smaller: fromChkpt,
					Larger:  toChkpt,
					Proof:   proof,
					Wrapped: err,
				}
			}
		}
	}
	return w.db.SetCheckpoint(logId, toChkpt)
}

// Registers a new log with the TOFU checkpoint.
// TODO provide a public key (or look it up using logId?) and check signature.
func (w *Witness) Register(logPK string, chkptRaw []byte) error {
	chkpt, err := w.verifyParse(chkptRaw)
	if err != nil {
		return err
	}
	return w.db.SetCheckpoint(logPK, chkpt)
}
