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
	"sort"
	"testing"

	"github.com/google/trillian/merkle/rfc6962/hasher"
	_ "github.com/mattn/go-sqlite3" // Load drivers for sqlite3
	"golang.org/x/mod/sumdb/note"
)

var (
	initChkpt = []byte("Log Checkpoint v0\n5\n41smjBUiAU70EtKlT6lIOIYtRTYxYXsDB+XHfcvu/BE=\n")
	newChkpt  = []byte("Log Checkpoint v0\n8\nV8K9aklZ4EPB+RMOk1/8VsJUdFZR77GDtZUQq84vSbo=\n")
	consProof = [][]byte{
		dh("b9e1d62618f7fee8034e4c5010f727ab24d8e4705cb296c374bf2025a87a10d2", 32),
		dh("aac66cd7a79ce4012d80762fe8eec3a77f22d1ca4145c3f4cee022e7efcd599d", 32),
		dh("89d0f753f66a290c483b39cd5e9eafb12021293395fad3d4a2ad053cfbcfdc9e", 32),
		dh("29e40bb79c966f4c6fe96aff6f30acfce5f3e8d84c02215175d6e018a5dee833", 32),
	}
	crInitChkpt = []byte("Log Checkpoint v0\n10\ne/S4liN8tioogdwxlCaXdQBb/5bxM9AWKA0bcZa9TXw=\n")
	crInitRange = [][]byte{
		dh("4e856ea495cf591cefb9eff66b311b2d5ec1d0901b8026909f88a3f126d9db11", 32),
		dh("3a357a5ff22c69641ff59c08ca67ccabdefdf317476501db8cafc73ebb5ff547", 32),
	}
	crNewChkpt = []byte("Log Checkpoint v0\n15\nsrKoB8sjvP1QAt1Ih3nqGHzjvmtRLs/geQdehrUHvqs=\n")
	crProof    = [][]byte{
		dh("ef626e0b64023948e57f34674c2574b3078c5af59a2faa095f4948736e8ca52e", 32),
		dh("8f75f7d88d3679ac6dd5a68a81215bfbeafe8c566b93013bbc82e64295628c8b", 32),
		dh("e034fb7af8223063c1c299ed91c11a0bc4cec15afd75e2abe4bb54c14d921ef0", 32),
	}
)

type LogOpts struct {
	ID         string
	PK         string
	useCompact bool
}

func signChkpts(skey string, chkpts []string) ([][]byte, error) {
	ns, err := note.NewSigner(skey)
	if err != nil {
		return nil, fmt.Errorf("signChkpt: couldn't create signer: %v", err)
	}
	signed := make([][]byte, len(chkpts))
	for i, c := range chkpts {
		s, err := note.Sign(&note.Note{Text: c}, ns)
		if err != nil {
			return nil, fmt.Errorf("signChkpt: couldn't sign note: %v", err)
		}
		signed[i] = s
	}
	return signed, nil
}

func newOpts(d *sql.DB, logs []LogOpts, wSK string) (Opts, error) {
	ns, err := note.NewSigner(wSK)
	if err != nil {
		return Opts{}, fmt.Errorf("newOpts: couldn't create a note signer: %v", err)
	}
	h := rfc6962.DefaultHasher
	logMap := make(map[string]LogInfo)
	for _, log := range logs {
		logV, err := note.NewVerifier(log.PK)
		if err != nil {
			return Opts{}, fmt.Errorf("newOpts: couldn't create a log verifier")
		}
		sigV := []note.Verifier{logV}
		logInfo := LogInfo{
			SigVs:      sigV,
			Hasher:     h,
			UseCompact: log.useCompact,
		}
		logMap[log.ID] = logInfo
	}
	opts := Opts{
		DB:        d,
		Signer:    ns,
		KnownLogs: logMap,
	}
	return opts, nil
}

func newOptsAndKeys(d *sql.DB, logs []LogOpts) (Opts, error) {
	wSK, _, err := note.GenerateKey(rand.Reader, "witness")
	if err != nil {
		return Opts{}, fmt.Errorf("couldn't generate witness keys")
	}
	return newOpts(d, logs, wSK)
}

func newWitness(t *testing.T, d *sql.DB, logs []LogOpts) *Witness {
	opts, err := newOptsAndKeys(d, logs)
	if err != nil {
		t.Fatalf("couldn't create witness opt struct: %v", err)
	}
	w, err := New(opts)
	if err != nil {
		t.Fatalf("couldn't create witness: %v", err)
	}
	return w
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

func mustCreateDB(t *testing.T) (*sql.DB, func() error) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open temporary in-memory DB: %v", err)
	}
	return db, db.Close
}

func TestGetLogs(t *testing.T) {
	for _, test := range []struct {
		desc   string
		logIDs []string
	}{
		{
			desc:   "no logs",
			logIDs: []string{},
		}, {
			desc:   "one log",
			logIDs: []string{"monkeys"},
		}, {
			desc:   "two logs",
			logIDs: []string{"bananas", "monkeys"},
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			d, closeFn := mustCreateDB(t)
			defer closeFn()
			ctx := context.Background()
			// Set up log keys and sign checkpoints.
			chkpts := make([][]byte, len(test.logIDs))
			logs := make([]LogOpts, len(test.logIDs))
			for i, logID := range test.logIDs {
				logSK, logPK, err := note.GenerateKey(rand.Reader, logID)
				if err != nil {
					t.Fatalf("couldn't generate log keys: %v", err)
				}
				logs[i] = LogOpts{ID: logID,
					PK:         logPK,
					useCompact: false,
				}
				signed, err := signChkpts(logSK, []string{string(initChkpt)})
				if err != nil {
					t.Fatalf("couldn't sign checkpoint: %v", err)
				}
				chkpts[i] = signed[0]
			}
			// Set up witness.
			w := newWitness(t, d, logs)
			// Update to a checkpoint for all logs.
			for i, logID := range test.logIDs {
				if _, err := w.Update(ctx, logID, chkpts[i], nil); err != nil {
					t.Errorf("failed to set checkpoint: %v", err)
				}
			}
			// Now see if the witness knows about these logs.
			knownLogs, err := w.GetLogs()
			if err != nil {
				t.Fatalf("couldn't get logs from witness: %v", err)
			}
			if len(knownLogs) != len(test.logIDs) {
				t.Fatalf("wanted %v logs but got %v", len(test.logIDs), len(knownLogs))
			}
			sort.Strings(knownLogs)
			for i := range knownLogs {
				if knownLogs[i] != test.logIDs[i] {
					t.Fatalf("wanted %v but got %v", knownLogs[i], test.logIDs[i])
				}
			}
		})
	}
}

func TestGetChkpt(t *testing.T) {
	for _, test := range []struct {
		desc      string
		setID     string
		queryID   string
		c         []byte
		wantThere bool
	}{
		{
			desc:      "happy path",
			setID:     "testlog",
			queryID:   "testlog",
			c:         initChkpt,
			wantThere: true,
		}, {
			desc:      "other log",
			setID:     "testlog",
			queryID:   "otherlog",
			c:         initChkpt,
			wantThere: false,
		}, {
			desc:      "nothing there",
			setID:     "testlog",
			queryID:   "testlog",
			c:         nil,
			wantThere: false,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			d, closeFn := mustCreateDB(t)
			defer closeFn()
			ctx := context.Background()
			// Set up log keys and sign checkpoint.
			logSK, logPK, err := note.GenerateKey(rand.Reader, test.setID)
			if err != nil {
				t.Fatalf("couldn't generate log keys: %v", err)
			}
			if test.c != nil {
				signed, err := signChkpts(logSK, []string{string(test.c)})
				if err != nil {
					t.Fatalf("couldn't sign checkpoint: %v", err)
				}
				test.c = signed[0]
			}
			// Set up witness keys and other parameters.
			wSK, wPK, err := note.GenerateKey(rand.Reader, "witness")
			if err != nil {
				t.Fatalf("couldn't generate witness keys: %v", err)
			}
			opts, err := newOpts(d, []LogOpts{{ID: test.setID,
				PK:         logPK,
				useCompact: false,
			}}, wSK)
			if err != nil {
				t.Fatalf("couldn't create witness opts: %v", err)
			}
			w, err := New(opts)
			if err != nil {
				t.Fatalf("couldn't create witness: %v", err)
			}
			// Set a checkpoint for the log if we want to for this test.
			if test.c != nil {
				if _, err := w.Update(ctx, test.setID, test.c, nil); err != nil {
					t.Errorf("failed to set checkpoint: %v", err)
				}
			}
			// Try to get the latest checkpoint.
			cosigned, err := w.GetCheckpoint(test.queryID)
			if !test.wantThere && err == nil {
				t.Fatalf("returned a checkpoint but shouldn't have")
			}
			// If we got something then verify it under the log and
			// witness public keys.
			if test.wantThere {
				if err != nil {
					t.Errorf("failed to get latest: %v", err)
				}
				wV, err := note.NewVerifier(wPK)
				if err != nil {
					t.Fatalf("couldn't create a witness verifier: %v", err)
				}
				logV, err := note.NewVerifier(logPK)
				if err != nil {
					t.Fatalf("couldn't create a log verifier: %v", err)
				}
				n, err := note.Open(cosigned, note.VerifierList(logV, wV))
				if err != nil {
					t.Fatalf("couldn't verify the co-signed checkpoint: %v", err)
				}
				if len(n.Sigs) != 2 {
					t.Fatalf("checkpoint doesn't verify under enough keys")
				}
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	for _, test := range []struct {
		desc     string
		initC    []byte
		initSize uint64
		newC     []byte
		pf       [][]byte
		useCR    bool
		initCR   [][]byte
		isGood   bool
	}{
		{
			desc:     "vanilla consistency happy path",
			initC:    initChkpt,
			initSize: 5,
			newC:     newChkpt,
			pf:       consProof,
			useCR:    false,
			isGood:   true,
		}, {
			desc:     "vanilla consistency smaller checkpoint",
			initC:    initChkpt,
			initSize: 5,
			newC:     []byte("Log Checkpoint v0\n4\nhashhashhash\n"),
			pf:       consProof,
			useCR:    false,
			isGood:   false,
		}, {
			desc:     "vanilla consistency garbage proof",
			initC:    initChkpt,
			initSize: 5,
			newC:     newChkpt,
			pf: [][]byte{
				dh("aaaa", 2),
				dh("bbbb", 2),
				dh("cccc", 2),
				dh("dddd", 2),
			},
			isGood: false,
		}, {
			desc:     "compact range happy path",
			initC:    crInitChkpt,
			initSize: 10,
			newC:     crNewChkpt,
			pf:       crProof,
			useCR:    true,
			initCR:   crInitRange,
			isGood:   true,
		}, {
			desc:     "compact range garbage proof",
			initC:    crInitChkpt,
			initSize: 10,
			newC:     crNewChkpt,
			pf: [][]byte{
				dh("aaaa", 2),
				dh("bbbb", 2),
				dh("cccc", 2),
				dh("dddd", 2),
			},
			useCR:  true,
			initCR: crInitRange,
			isGood: false,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			d, closeFn := mustCreateDB(t)
			defer closeFn()
			ctx := context.Background()
			// Set up log keys and sign checkpoints.
			logID := "testlog"
			logSK, logPK, err := note.GenerateKey(rand.Reader, logID)
			if err != nil {
				t.Fatalf("couldn't generate log keys: %v", err)
			}
			signed, err := signChkpts(logSK, []string{string(test.initC), string(test.newC)})
			if err != nil {
				t.Fatalf("couldn't sign checkpoint: %v", err)
			}
			test.initC = signed[0]
			test.newC = signed[1]
			// Set up witness.
			w := newWitness(t, d, []LogOpts{{ID: logID,
				PK:         logPK,
				useCompact: test.useCR}})
			// Set an initial checkpoint for the log.
			if _, err := w.Update(ctx, logID, test.initC, test.initCR); err != nil {
				t.Errorf("failed to set checkpoint: %v", err)
			}
			// Now update from this checkpoint to a newer one.
			size, err := w.Update(ctx, logID, test.newC, test.pf)
			if test.isGood {
				if err != nil {
					t.Fatalf("can't update to new checkpoint: %v", err)
				}
				if size != test.initSize {
					t.Fatal("witness returned the wrong size in updating")
				}
			} else {
				if err == nil {
					t.Fatal("should have gotten an error but didn't")
				}
			}
		})
	}
}
