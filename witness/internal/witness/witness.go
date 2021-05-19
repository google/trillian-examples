package witness

import (
	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/merkle/logverifier"
)

type Witness struct {
	db       *Database
	Hasher   hashers.LogHasher
	Verifier logverifier.LogVerifier
}

// NewWitness creates a new witness, which initially has no logs to follow.
func NewWitness(h hashers.LogHasher, db *Database) *Witness {
	return &Witness{
		db:       db,
		Hasher:   h,
		Verifier: logverifier.New(h),
	}
}

// GetLatest gets the latest checkpoint for a given log, which should be
// consistent with all other checkpoints for the same log.
func (w *Witness) GetLatest(logId string) (*log.Checkpoint, error) {
	return w.db.GetLatest(logId)
}

// Update updates latest checkpoint if `to` matches the current version (`from`).
// TODO check signatures on both checkpoints.
func (w *Witness) Update(logId string, from, to log.Checkpoint, proof log.Proof) error {
	latest, err := w.db.GetLatest(logId)
	if err != nil {
		return err
	}
	// Don't want consistency proofs with respect to older checkpoints.
	if from.Size < latest.Size {
		return ErrStale{
			Smaller: from,
			Latest:  latest,
		}
	}
	//if from, err != w.db.GetCheckpoint() {
	//	return ErrBad{}
	//}
	if from.Size > 0 {
		if to.Size > from.Size {
			if err := w.Verifier.VerifyConsistencyProof(int64(from.Size), int64(to.Size), from.RootHash, to.RootHash, proof); err != nil {
				return ErrInconsistency{
					Smaller: from,
					Larger:  to,
					Proof:   proof,
					Wrapped: err,
				}
			}
		}
	}
	return w.db.SetCheckpoint(logId, to)
}

// Registers a new log with the TOFU checkpoint.
// TODO provide a public key (or look it up using logId?) and check signature.
func (w *Witness) Init(logId string, chkpt log.Checkpoint) error {
	return w.db.SetCheckpoint(logId, chkpt)
}
