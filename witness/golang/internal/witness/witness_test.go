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
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/google/trillian/merkle/logverifier"
	"github.com/google/trillian/merkle/rfc6962/hasher"
	_ "github.com/mattn/go-sqlite3" // Load drivers for sqlite3
	"golang.org/x/mod/sumdb/note"
)

var (
	initC = Chkpt{
		Size: 5,
		Raw:  []byte("Log Checkpoint v0\n5\n41smjBUiAU70EtKlT6lIOIYtRTYxYXsDB+XHfcvu/BE=\n"),
	}
	newC = Chkpt{
		Size: 8,
		Raw:  []byte("Log Checkpoint v0\n8\nV8K9aklZ4EPB+RMOk1/8VsJUdFZR77GDtZUQq84vSbo=\n"),
	}
	consProof = [][]byte{
		dh("b9e1d62618f7fee8034e4c5010f727ab24d8e4705cb296c374bf2025a87a10d2", 32),
		dh("aac66cd7a79ce4012d80762fe8eec3a77f22d1ca4145c3f4cee022e7efcd599d", 32),
		dh("89d0f753f66a290c483b39cd5e9eafb12021293395fad3d4a2ad053cfbcfdc9e", 32),
		dh("29e40bb79c966f4c6fe96aff6f30acfce5f3e8d84c02215175d6e018a5dee833", 32),
	}
)

func signChkpts(skey string, chkpts []string) ([][]byte, error) {
	ns, err := note.NewSigner(skey)
	if err != nil {
		return nil, fmt.Errorf("signChkpt: couldn't create signer")
	}
	signed := make([][]byte, len(chkpts))
	for i, c := range chkpts {
		s, err := note.Sign(&note.Note{Text: c}, ns)
		if err != nil {
			return nil, fmt.Errorf("signChkpt: couldn't sign note")
		}
		signed[i] = s
	}
	return signed, nil
}

func setOpts(d *Database, logID string, logPK string, wSK string) (*Opts, error) {
	ns, err := note.NewSigner(wSK)
	if err != nil {
		return nil, fmt.Errorf("setOpts: couldn't create a note signer")
	}
	h := hasher.DefaultHasher
	logV, err := note.NewVerifier(logPK)
	if err != nil {
		return nil, fmt.Errorf("setOpts: couldn't create a log verifier")
	}
	sigVs := []note.Verifier{logV}
	log := LogInfo{
		Hasher: h,
		SigVs:  sigVs,
		LogV:   logverifier.New(h),
	}
	m := map[string]LogInfo{logID: log}
	opts := &Opts{
		Storage:   d,
		Signer:    ns,
		KnownLogs: m,
	}
	return opts, nil
}

func setOptsAndKeys(d *Database, logID string, logPK string) (*Opts, error) {
	wSK, _, err := note.GenerateKey(rand.Reader, "witness")
	if err != nil {
		return nil, fmt.Errorf("couldn't generate witness keys")
	}
	return setOpts(d, logID, logPK, wSK)
}

func setWitness(t *testing.T, d *Database, logID string, logPK string) *Witness {
	opts, err := setOptsAndKeys(d, logID, logPK)
	if err != nil {
		t.Fatalf("couldn't create witness opt struct: %v", err)
	}
	return New(opts)
}

// dh is taken from https://github.com/google/trillian/blob/master/merkle/logverifier/log_verifier_test.go.
func dh(h string, expLen int) []byte {
	r, err := hex.DecodeString(h)
	if err != nil {
		panic(err)
	}
	if got := len(r); got != expLen {
		panic(fmt.Sprintf("decode %q: len=%d, want %d", h, got, expLen))
	}
	return r
}

func mustCreateDB(t *testing.T) (*Database, func() error) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open temporary in-memory DB: %v", err)
	}
	d, err := NewDatabase(db)
	if err != nil {
		t.Fatalf("failed to create DB: %v", err)
	}
	return d, db.Close
}

func TestGoodGetChkpt(t *testing.T) {
	d, closeFn := mustCreateDB(t)
	defer closeFn()
	ctx := context.Background()
	// Set up log keys and sign checkpoint.
	logID := "testlog"
	logSK, logPK, err := note.GenerateKey(rand.Reader, logID)
	if err != nil {
		t.Errorf("couldn't generate log keys: %v", err)
	}
	signed, err := signChkpts(logSK, []string{string(initC.Raw)})
	if err != nil {
		t.Errorf("couldn't sign checkpoint: %v", err)
	}
	initC.Raw = signed[0]
	// Set up witness keys and other parameters.
	wSK, wPK, err := note.GenerateKey(rand.Reader, "witness")
	if err != nil {
		t.Errorf("couldn't generate witness keys: %v", err)
	}
	opts, err := setOpts(d, logID, logPK, wSK)
	if err != nil {
		t.Errorf("couldn't create witness opts: %v", err)
	}
	w := New(opts)
	// Set an initial checkpoint for the log (using the database directly).
	if err := d.SetCheckpoint(ctx, logID, 0, &initC); err != nil {
		t.Errorf("failed to set checkpoint: %v", err)
	}
	// Get the latest checkpoint and make sure it's signed properly by both
	// the witness and the log.
	cosigned, err := w.GetCheckpoint(logID)
	if err != nil {
		t.Errorf("failed to get latest: %v", err)
	}
	wV, err := note.NewVerifier(wPK)
	if err != nil {
		t.Errorf("couldn't create a witness verifier: %v", err)
	}
	logV, err := note.NewVerifier(logPK)
	if err != nil {
		t.Errorf("couldn't create a log verifier: %v", err)
	}
	n, err := note.Open(cosigned, note.VerifierList(logV, wV))
	if err != nil {
		t.Errorf("couldn't verify the co-signed checkpoint: %v", err)
	}
	if len(n.Sigs) != 2 {
		t.Fatalf("checkpoint doesn't verify under enough keys")
	}
}

func TestGoodUpdate(t *testing.T) {
	d, closeFn := mustCreateDB(t)
	defer closeFn()
	ctx := context.Background()
	// Set up log keys and sign checkpoints.
	logID := "testlog"
	logSK, logPK, err := note.GenerateKey(rand.Reader, logID)
	if err != nil {
		t.Errorf("couldn't generate log keys: %v", err)
	}
	signed, err := signChkpts(logSK, []string{string(initC.Raw), string(newC.Raw)})
	if err != nil {
		t.Errorf("couldn't sign checkpoint: %v", err)
	}
	initC.Raw = signed[0]
	newC.Raw = signed[1]
	// Set up witness.
	w := setWitness(t, d, logID, logPK)

	// Set an initial checkpoint for the log (using the database directly).
	if err := d.SetCheckpoint(ctx, logID, 0, &initC); err != nil {
		t.Errorf("failed to set checkpoint: %v", err)
	}
	// Now update from this checkpoint to a newer one.
	size, err := w.Update(ctx, logID, initC.Size, newC.Raw, consProof)
	if err != nil {
		t.Fatalf("can't update to new checkpoint: %v", err)
	}
	if size != initC.Size {
		t.Fatal("witness returned the wrong size in updating")
	}
}

// This should fail because there are no checkpoints stored at all.
func TestGetChkptNoneThere(t *testing.T) {
	d, closeFn := mustCreateDB(t)
	defer closeFn()
	// Set up log keys.
	logID := "testlog"
	_, logPK, err := note.GenerateKey(rand.Reader, logID)
	if err != nil {
		t.Errorf("couldn't generate log keys: %v", err)
	}
	w := setWitness(t, d, logID, logPK)
	// Get the latest checkpoint for the log, which shouldn't be there.
	if _, err = w.GetCheckpoint(logID); err == nil {
		t.Fatal("got a checkpoint but shouldn't have")
	}
}

// This should fail because the stored checkpoint is for a different log.
func TestGetChkptOtherLog(t *testing.T) {
	d, closeFn := mustCreateDB(t)
	defer closeFn()
	ctx := context.Background()
	// Set up log keys and sign checkpoint.
	logID := "testlog"
	logSK, logPK, err := note.GenerateKey(rand.Reader, logID)
	if err != nil {
		t.Errorf("couldn't generate log keys: %v", err)
	}
	signed, err := signChkpts(logSK, []string{string(initC.Raw)})
	if err != nil {
		t.Error("couldn't sign checkpoint", err)
	}
	initC.Raw = signed[0]
	w := setWitness(t, d, logID, logPK)
	// Set an initial checkpoint for the log (using the database directly).
	if err := d.SetCheckpoint(ctx, logID, 0, &initC); err != nil {
		t.Error("failed to set checkpoint", err)
	}
	// Get the latest checkpoint for a different log, which shouldn't be
	// there.
	if _, err = w.GetCheckpoint("other log"); err == nil {
		t.Fatal("got a checkpoint but shouldn't have")
	}
}

// This should fail because the consistency proof is bad.
func TestUpdateBadProof(t *testing.T) {
	badProof := [][]byte{
		dh("aaaa", 2),
		dh("bbbb", 2),
		dh("cccc", 2),
		dh("dddd", 2),
	}
	d, closeFn := mustCreateDB(t)
	defer closeFn()
	ctx := context.Background()
	// Set up log keys and sign checkpoints.
	logID := "testlog"
	logSK, logPK, err := note.GenerateKey(rand.Reader, logID)
	if err != nil {
		t.Errorf("couldn't generate log keys: %v", err)
	}
	signed, err := signChkpts(logSK, []string{string(initC.Raw), string(newC.Raw)})
	if err != nil {
		t.Errorf("couldn't sign checkpoint: %v", err)
	}
	initC.Raw = signed[0]
	newC.Raw = signed[1]
	// Set up witness.
	w := setWitness(t, d, logID, logPK)

	// Set an initial checkpoint for the log (using the database directly).
	if err := d.SetCheckpoint(ctx, logID, 0, &initC); err != nil {
		t.Errorf("failed to set checkpoint: %v", err)
	}
	// Now update from this checkpoint to a newer one.
	if _, err = w.Update(ctx, logID, initC.Size, newC.Raw, badProof); err == nil {
		t.Fatal("updated to new checkpoint but shouldn't have")
	}
}

// This should fail because the caller is using the wrong size when updating.
func TestUpdateStale(t *testing.T) {
	d, closeFn := mustCreateDB(t)
	defer closeFn()
	ctx := context.Background()
	// Set up log keys and sign checkpoints.
	logID := "testlog"
	logSK, logPK, err := note.GenerateKey(rand.Reader, logID)
	if err != nil {
		t.Errorf("couldn't generate log keys: %v", err)
	}
	signed, err := signChkpts(logSK, []string{string(initC.Raw), string(newC.Raw)})
	if err != nil {
		t.Errorf("couldn't sign checkpoint: %v", err)
	}
	initC.Raw = signed[0]
	newC.Raw = signed[1]
	// Set up witness.
	w := setWitness(t, d, logID, logPK)

	// Set an initial checkpoint for the log (using the database directly).
	if err := d.SetCheckpoint(ctx, logID, 0, &initC); err != nil {
		t.Errorf("failed to set checkpoint: %v", err)
	}
	// Now update from this checkpoint to a newer one but using the wrong
	// size for the latest one.
	size, err := w.Update(ctx, logID, initC.Size-1, newC.Raw, consProof)
	if err != nil {
		t.Errorf("failed in updating: %v", err)
	}
	if size != initC.Size {
		t.Fatal("witness returned the wrong size in updating")
	}
}
