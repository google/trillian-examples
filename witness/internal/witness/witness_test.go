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
	//"bytes"
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

func signChkpt(skey string, chkpt string) ([]byte, error) {
	ns, err := note.NewSigner(skey)
	if err != nil {
		return nil, fmt.Errorf("signChkpt: couldn't create signer")
	}
	signed, err := note.Sign(&note.Note{Text: chkpt}, ns)
	if err != nil {
		return nil, fmt.Errorf("signChkpt: couldn't sign note")
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

func TestHappyPaths(t *testing.T) {
	initC := Chkpt{
		Size: 5,
		Raw:  []byte("Log Checkpoint v0\n5\n41smjBUiAU70EtKlT6lIOIYtRTYxYXsDB+XHfcvu/BE=\n"),
	}
	newC := Chkpt{
		Size: 8,
		Raw:  []byte("Log Checkpoint v0\n8\nV8K9aklZ4EPB+RMOk1/8VsJUdFZR77GDtZUQq84vSbo=\n"),
	}
	consProof := [][]byte{
		//log.Proof{
		dh("b9e1d62618f7fee8034e4c5010f727ab24d8e4705cb296c374bf2025a87a10d2", 32),
		dh("aac66cd7a79ce4012d80762fe8eec3a77f22d1ca4145c3f4cee022e7efcd599d", 32),
		dh("89d0f753f66a290c483b39cd5e9eafb12021293395fad3d4a2ad053cfbcfdc9e", 32),
		dh("29e40bb79c966f4c6fe96aff6f30acfce5f3e8d84c02215175d6e018a5dee833", 32),
	}

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Error("failed to open temporary in-memory DB", err)
	}
	defer db.Close()

	d, err := NewDatabase(db)
	if err != nil {
		t.Error("failed to create DB", err)
	}

	ctx := context.Background()
	// Set up log keys.
	logID := "testlog"
	logSK, logPK, err := note.GenerateKey(rand.Reader, logID)
	if err != nil {
		t.Error("couldn't generate log keys", err)
	}
	// Have the log sign both checkpoints.
	initC.Raw, err = signChkpt(logSK, string(initC.Raw))
	if err != nil {
		t.Error("couldn't sign checkpoint", err)
	}
	newC.Raw, err = signChkpt(logSK, string(newC.Raw))
	if err != nil {
		t.Error("couldn't sign checkpoint", err)
	}
	// Set up witness keys and other parameters.
	wSK, wPK, err := note.GenerateKey(rand.Reader, "witness")
	if err != nil {
		t.Error("couldn't generate witness keys", err)
	}
	opts, err := setOpts(d, logID, logPK, wSK)
	if err != nil {
		t.Error("couldn't create witness opts", err)
	}
	w := NewWitness(opts)

	// Set an initial checkpoint for the log (using the database directly).
	if err := d.SetCheckpoint(ctx, logID, nil, &initC); err != nil {
		t.Error("failed to set checkpoint", err)
	}
	// Get the latest checkpoint and make sure it's signed properly by both
	// the witness and the log.
	cosigned, err := w.GetCheckpoint(logID)
	if err != nil {
		t.Error("failed to get latest", err)
	}
	wV, err := note.NewVerifier(wPK)
	if err != nil {
		t.Error("couldn't create a witness verifier")
	}
	logV, err := note.NewVerifier(logPK)
	if err != nil {
		t.Error("couldn't create a log verifier")
	}
	n, err := note.Open(cosigned, note.VerifierList(logV, wV))
	if err != nil {
		t.Error("couldn't verify the co-signed checkpoint", err)
	}
	if len(n.Sigs) != 2 {
		t.Fatalf("checkpoint doesn't verify under enough keys")
	}
	// Now update from this checkpoint to a newer one.
	err = w.Update(ctx, logID, newC.Raw, consProof)
	if err != nil {
		t.Fatal("can't update to new checkpoint", err)
	}
}
