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
	"encoding/hex"
	"fmt"
	"sort"
	"testing"

	"github.com/google/trillian-examples/witness/golang/internal/persistence/inmemory"
	_ "github.com/mattn/go-sqlite3" // Load drivers for sqlite3
	"github.com/transparency-dev/merkle/rfc6962"
	"golang.org/x/mod/sumdb/note"
)

var (
	// https://play.golang.org/p/PKcyNC2sjBL to regenerate any messages if needed.
	mPK       = "monkeys+db4d9f7e+AULaJMvTtDLHPUcUrjdDad9vDlh/PTfC2VV60JUtCfWT"
	mSK       = "PRIVATE+KEY+monkeys+db4d9f7e+ATWIAF3yVBG+Hv1rZFQoNt/BaURkLPtOFMAM2HrEeIr6"
	bPK       = "bananas+cf639f13+AaPjhFnPCQnid/Ql32KWhmh+uk72FVRfK+2DLmO3BI3M"
	bSK       = "PRIVATE+KEY+bananas+cf639f13+AdjzytHoXdvn+1vG2UXXqFR3LZ+kvnmQZFretRaKfTIu"
	wPK       = "witness+f13a86db+AdYV1Ztajd9BvyjP2HgpwrqYL6TjOwIjGMOq8Bu42xbN"
	wSK       = "PRIVATE+KEY+witness+f13a86db+AaLa/dfyBhyo/m0Z7WCi98ENVZWtrP8pxgRNrx7tIWiA"
	mInit     = []byte("Log Checkpoint v0\n5\n41smjBUiAU70EtKlT6lIOIYtRTYxYXsDB+XHfcvu/BE=\n\n— monkeys 202ffu1mJ0hrPIodgN0YQV7uZk9CAEzh+hOQyRjkf9CDY1IOr/oiH6WkzGIy7aEF0rw/9SHiZztjDiTOb53i6CE/zQ8=\n")
	bInit     = []byte("Log Checkpoint v0\n5\n41smjBUiAU70EtKlT6lIOIYtRTYxYXsDB+XHfcvu/BE=\n\n— bananas z2OfE18+NwUjjJBXH7m+fh67bu29p1Jbypr4GFUQohgQgCeuPJZtGTvfR9Pquh2Iebfq+6bhl3G/77lsKiGIea6NAwE=\n")
	mNext     = []byte("Log Checkpoint v0\n8\nV8K9aklZ4EPB+RMOk1/8VsJUdFZR77GDtZUQq84vSbo=\n\n— monkeys 202ffm1n5/h7F3x0D6CCysNnqiL2WBu7iubseJggohhX6JBbeLts5NWKY/g0r6yNRVN/k+QJJx1Sii0MzbMUTnmO+wA=\n")
	consProof = [][]byte{
		dh("b9e1d62618f7fee8034e4c5010f727ab24d8e4705cb296c374bf2025a87a10d2", 32),
		dh("aac66cd7a79ce4012d80762fe8eec3a77f22d1ca4145c3f4cee022e7efcd599d", 32),
		dh("89d0f753f66a290c483b39cd5e9eafb12021293395fad3d4a2ad053cfbcfdc9e", 32),
		dh("29e40bb79c966f4c6fe96aff6f30acfce5f3e8d84c02215175d6e018a5dee833", 32),
	}
	crInit      = []byte("Log Checkpoint v0\n10\ne/S4liN8tioogdwxlCaXdQBb/5bxM9AWKA0bcZa9TXw=\n\n— monkeys 202ffo2uuOa4Vo94IaAdawszJz+eyJRyK46Yp9K5tz7qREYFqb4D9WGsZHsM5tuc6jKVTnldCSCGJk/FwGUv/IACjwo=\n")
	crInitRange = [][]byte{
		dh("4e856ea495cf591cefb9eff66b311b2d5ec1d0901b8026909f88a3f126d9db11", 32),
		dh("3a357a5ff22c69641ff59c08ca67ccabdefdf317476501db8cafc73ebb5ff547", 32),
	}
	crNext  = []byte("Log Checkpoint v0\n15\nsrKoB8sjvP1QAt1Ih3nqGHzjvmtRLs/geQdehrUHvqs=\n\n— monkeys 202ffsI0bYEfiyoXANzIXNoUCcro6D4TfvQlU+zbxmJLGLXPGu15VNIWBOJr0EUQO0P3dxSJI5SsFLLYQLyqvRercQ4=\n")
	crProof = [][]byte{
		dh("ef626e0b64023948e57f34674c2574b3078c5af59a2faa095f4948736e8ca52e", 32),
		dh("8f75f7d88d3679ac6dd5a68a81215bfbeafe8c566b93013bbc82e64295628c8b", 32),
		dh("e034fb7af8223063c1c299ed91c11a0bc4cec15afd75e2abe4bb54c14d921ef0", 32),
	}

	_ = mSK
	_ = bSK
)

const logOrigin = "Log Checkpoint v0"

type logOpts struct {
	ID         string
	origin     string
	PK         string
	useCompact bool
}

func newWitness(t *testing.T, logs []logOpts) *Witness {
	// Set up Opts for the witness.
	ns, err := note.NewSigner(wSK)
	if err != nil {
		t.Fatalf("couldn't create a witness signer: %v", err)
	}
	h := rfc6962.DefaultHasher
	logMap := make(map[string]LogInfo)
	for _, log := range logs {
		logV, err := note.NewVerifier(log.PK)
		if err != nil {
			t.Fatalf("couldn't create a log verifier: %v", err)
		}
		logInfo := LogInfo{
			Origin:     log.origin,
			SigV:       logV,
			Hasher:     h,
			UseCompact: log.useCompact,
		}
		logMap[log.ID] = logInfo
	}
	opts := Opts{
		Persistence: inmemory.NewPersistence(),
		Signer:      ns,
		KnownLogs:   logMap,
	}
	// Create the witness
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

func TestGetLogs(t *testing.T) {
	for _, test := range []struct {
		desc   string
		logIDs []string
		logPKs []string
		chkpts [][]byte
	}{
		{
			desc:   "no logs",
			logIDs: []string{},
		}, {
			desc:   "one log",
			logIDs: []string{"monkeys"},
			logPKs: []string{mPK},
			chkpts: [][]byte{mInit},
		}, {
			desc:   "two logs",
			logIDs: []string{"bananas", "monkeys"},
			logPKs: []string{bPK, mPK},
			chkpts: [][]byte{bInit, mInit},
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ctx := context.Background()
			// Set up witness.
			logs := make([]logOpts, len(test.logIDs))
			for i, logID := range test.logIDs {
				logs[i] = logOpts{
					ID:         logID,
					origin:     logOrigin,
					PK:         test.logPKs[i],
					useCompact: false,
				}
			}
			w := newWitness(t, logs)
			// Update to a checkpoint for all logs.
			for i, logID := range test.logIDs {
				if _, err := w.Update(ctx, logID, test.chkpts[i], nil); err != nil {
					t.Errorf("failed to set checkpoint: %v", err)
				}
			}
			// Now see if the witness knows about these logs.
			knownLogs, err := w.GetLogs()
			if err != nil {
				t.Fatalf("couldn't get logs from witness: %v", err)
			}
			if len(knownLogs) != len(test.logIDs) {
				t.Fatalf("got %d logs, want %d", len(knownLogs), len(test.logIDs))
			}
			sort.Strings(knownLogs)
			for i := range knownLogs {
				if knownLogs[i] != test.logIDs[i] {
					t.Fatalf("got %q, want %q", test.logIDs[i], knownLogs[i])
				}
			}
		})
	}
}

func TestGetChkpt(t *testing.T) {
	for _, test := range []struct {
		desc      string
		setID     string
		setPK     string
		queryID   string
		queryPK   string
		c         []byte
		wantThere bool
	}{
		{
			desc:      "happy path",
			setID:     "monkeys",
			setPK:     mPK,
			queryID:   "monkeys",
			queryPK:   mPK,
			c:         mInit,
			wantThere: true,
		}, {
			desc:      "other log",
			setID:     "monkeys",
			setPK:     mPK,
			queryID:   "bananas",
			queryPK:   bPK,
			c:         mInit,
			wantThere: false,
		}, {
			desc:      "nothing there",
			setID:     "monkeys",
			setPK:     mPK,
			queryID:   "monkeys",
			queryPK:   mPK,
			c:         nil,
			wantThere: false,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ctx := context.Background()
			// Set up witness.
			w := newWitness(t, []logOpts{{
				ID:         test.setID,
				origin:     logOrigin,
				PK:         test.setPK,
				useCompact: false}})
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
				logV, err := note.NewVerifier(test.queryPK)
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
			initC:    mInit,
			initSize: 5,
			newC:     mNext,
			pf:       consProof,
			useCR:    false,
			isGood:   true,
		},
		{
			desc:     "vanilla path, but the first line changed",
			initC:    mInit,
			initSize: 5,
			newC:     []byte("Frog Checkpoint v0\n8\nV8K9aklZ4EPB+RMOk1/8VsJUdFZR77GDtZUQq84vSbo=\n\n— monkeys 202ffgCVdfZmrroccRdQoEfn2TfmXHez4R++GvVrFvFiaI85O12aTV5GpNOvWsuQW77eNxQ2b7ggYeglzF/QSy/EBws=\n"),
			pf:       consProof,
			useCR:    false,
			isGood:   false,
		}, {
			desc:     "vanilla consistency smaller checkpoint",
			initC:    mNext,
			initSize: 8,
			newC:     mInit,
			pf:       consProof,
			useCR:    false,
			isGood:   false,
		}, {
			desc:     "vanilla consistency garbage proof",
			initC:    mInit,
			initSize: 5,
			newC:     mNext,
			pf: [][]byte{
				dh("aaaa", 2),
				dh("bbbb", 2),
				dh("cccc", 2),
				dh("dddd", 2),
			},
			isGood: false,
		}, {
			desc:     "compact range happy path",
			initC:    crInit,
			initSize: 10,
			newC:     crNext,
			pf:       crProof,
			useCR:    true,
			initCR:   crInitRange,
			isGood:   true,
		}, {
			desc:     "compact range garbage proof",
			initC:    crInit,
			initSize: 10,
			newC:     crNext,
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
			ctx := context.Background()
			logID := "monkeys"
			// Set up witness.
			w := newWitness(t, []logOpts{{
				ID:         logID,
				origin:     logOrigin,
				PK:         mPK,
				useCompact: test.useCR}})
			// Set an initial checkpoint for the log.
			if _, err := w.Update(ctx, logID, test.initC, test.initCR); err != nil {
				t.Errorf("failed to set checkpoint: %v", err)
			}
			// Now update from this checkpoint to a newer one.
			_, err := w.Update(ctx, logID, test.newC, test.pf)
			if test.isGood {
				if err != nil {
					t.Fatalf("can't update to new checkpoint: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("should have gotten an error but didn't")
				}
			}
		})
	}
}
