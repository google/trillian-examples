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

// The github package provides support for pushing witnessed checkpoints to a
// github actions based distributor.
package github

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/glog"

	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/internal/github"
	"github.com/google/trillian-examples/serverless/config"
	"golang.org/x/mod/sumdb/note"
)

// Log represents a log known to the distributor.
type Log struct {
	Config config.Log
	// SigV can verify the source log signatures.
	SigV note.Verifier
}

// Witness describes the operations the feeder needs to interact with a witness.
type Witness interface {
	// GetLatestCheckpoint returns the latest checkpoint the witness holds for the given logID.
	// Must return os.ErrNotExists if the logID is known, but it has no checkpoint for that log.
	GetLatestCheckpoint(ctx context.Context, logID string) ([]byte, error)
}

// DistributeOptions contains the various configuration and state required to perform a distribute action.
type DistributeOptions struct {
	// Repo is the repository containing the distributor.
	Repo github.Repository
	// DistributorPath specifies the path to the root directory of the distributor data in the repo.
	DistributorPath string

	// Logs identifies the list of source logs whose checkpoints are being distributed.
	Logs []Log

	// WitSigV can verify the cosignatures from the witness we're distributing from.
	WitSigV note.Verifier
	// Witness is a client for talking to the witness we're distributing from.
	Witness Witness
}

// DistributeOnce a) polls the witness b) updates the fork c) proposes a PR if needed.
func DistributeOnce(ctx context.Context, opts *DistributeOptions) error {
	numErrs := 0
	for _, log := range opts.Logs {
		if err := distributeForLog(ctx, log, opts); err != nil {
			glog.Warningf("Failed to distribute %q (%s): %v", opts.Repo, log.SigV.Name(), err)
			numErrs++
		}
	}
	if numErrs > 0 {
		return fmt.Errorf("failed to distribute %d out of %d logs", numErrs, len(opts.Logs))
	}
	return nil
}

func distributeForLog(ctx context.Context, l Log, opts *DistributeOptions) error {
	// Ensure the branch exists for us to use to raise a PR
	wl := strings.Map(safeBranchChars, fmt.Sprintf("%s_%s", opts.WitSigV.Name(), l.SigV.Name()))
	witnessBranch := fmt.Sprintf("witness_%s", wl)
	if err := opts.Repo.CreateOrUpdateBranch(ctx, witnessBranch); err != nil {
		return fmt.Errorf("failed to create witness branch %q: %v", witnessBranch, err)
	}
	// This will be used on both the witness and the distributor.
	// At the moment the ID is arbitrary and is up to the discretion of the operators
	// of these parties. We should address this. If we don't manage to do so in time,
	// we'll need to allow this ID to be configured separately for each entity.
	logID := l.Config.ID

	wRaw, err := opts.Witness.GetLatestCheckpoint(ctx, logID)
	if err != nil {
		return err
	}
	wCp, wcpRaw, witnessNote, err := log.ParseCheckpoint(wRaw, l.Config.Origin, l.SigV, opts.WitSigV)
	if err != nil {
		return fmt.Errorf("couldn't parse witnessed checkpoint: %v", err)
	}
	if nWitSigs, want := len(witnessNote.Sigs)-1, 1; nWitSigs != want {
		return fmt.Errorf("checkpoint has %d witness sigs, want %d", nWitSigs, want)
	}

	logDir := filepath.Join(opts.DistributorPath, "logs", logID)
	found, err := alreadyPresent(ctx, witnessNote.Text, logDir, opts.Repo, opts.WitSigV)
	if err != nil {
		return fmt.Errorf("couldn't determine whether to distribute: %v", err)
	}
	if found {
		glog.V(1).Infof("%q (%s): CP already present in distributor, not raising PR.", opts.Repo, l.SigV.Name())
		return nil
	}

	outputPath := filepath.Join(logDir, "incoming", fmt.Sprintf("checkpoint_%s", wcpID(wcpRaw, opts.WitSigV.Name())))
	// First, check whether we've already managed to submit this CP into the incoming directory
	if _, err := opts.Repo.ReadFile(ctx, outputPath); err == nil {
		return fmt.Errorf("witnessed checkpoint already pending: %v", outputPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to check for existing pending checkpoint: %v", err)
	}

	msg := fmt.Sprintf("Witness checkpoint@%v", wCp.Size)
	if err := opts.Repo.CommitFile(ctx, outputPath, wRaw, witnessBranch, msg); err != nil {
		return fmt.Errorf("failed to commit updated checkpoint.witnessed file: %v", err)
	}

	glog.V(1).Infof("%q (%s): Creating PR", opts.Repo, l.SigV.Name())
	return opts.Repo.CreatePR(ctx, fmt.Sprintf("[Witness] %s: %s@%d", opts.WitSigV.Name(), wCp.Origin, wCp.Size), witnessBranch)
}

// wcpID returns a stable identifier for a given checkpoint for a given witness ID.
// The witness ID allows multiple users to upload their witnessed view of the same
// checkpoint without git collisions, but means that the same user trying multiple
// times won't create multiple files.
func wcpID(r []byte, witnessID string) string {
	h := sha256.Sum256(append(r, []byte(witnessID)...))
	return hex.EncodeToString(h[:])
}

// safeBranchChars maps runes to a small set of runes suitable for use in a github branch name.
func safeBranchChars(i rune) rune {
	if (i >= '0' && i <= '9') ||
		(i >= 'a' && i <= 'z') ||
		(i >= 'A' && i <= 'Z') ||
		i == '-' {
		return i
	}
	return '_'
}

// alreadyPresent determines if a given checkpoint is already known to the distributor.
func alreadyPresent(ctx context.Context, cpBody string, logDir string, repo github.Repository, wSigV note.Verifier) (bool, error) {
	for i := 0; ; i++ {
		cpPath := filepath.Join(logDir, fmt.Sprintf("checkpoint.%d", i))
		cpRaw, err := repo.ReadFile(ctx, cpPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// We've reached the end of the list of checkpoints without
				// encountering our checkpoint, so send it!
				return false, nil
			}
			return false, fmt.Errorf("failed to read %q: %v", cpPath, err)
		}
		n, err := note.Open(cpRaw, note.VerifierList(wSigV))
		if err != nil {
			if _, ok := err.(*note.UnverifiedNoteError); ok {
				// Not signed by us, ignore it.
				continue
			}
			return false, fmt.Errorf("failed to open %q: %v", cpPath, err)
		}
		if n.Text == cpBody {
			// We've found our candidate CP and it's already signed by us, no need to send.
			return true, nil
		}
	}
}
