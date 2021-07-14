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

package checkpoints

import (
	"testing"

	"golang.org/x/mod/sumdb/note"
)

func TestCombine(t *testing.T) {
	logS, logV := genKeyPair(t, "log")
	wit1S, wit1V := genKeyPair(t, "w1")
	wit2S, wit2V := genKeyPair(t, "w2")
	wit3S, wit3V := genKeyPair(t, "w3")

	for _, test := range []struct {
		desc         string
		logSigV      note.Verifier
		witnessSigVs note.Verifiers
		cps          [][]byte
		wantSigVs    []note.Verifier
		wantErr      bool
	}{
		{
			desc:         "works - first CP has all the signatures",
			logSigV:      logV,
			witnessSigVs: note.VerifierList(wit1V, wit2V, wit3V),
			cps: [][]byte{
				newCP(t, "body", logS, wit1S, wit2S, wit3S),
				newCP(t, "body", logS),
			},
			wantSigVs: []note.Verifier{logV, wit1V, wit2V, wit3V},
		}, {
			desc:         "works",
			logSigV:      logV,
			witnessSigVs: note.VerifierList(wit1V, wit2V, wit3V),
			cps: [][]byte{
				newCP(t, "body", logS),
				newCP(t, "body", logS, wit1S),
				newCP(t, "body", logS, wit2S),
				newCP(t, "body", logS, wit3S),
			},
			wantSigVs: []note.Verifier{logV, wit1V, wit2V, wit3V},
		}, {
			desc:         "many dupes",
			logSigV:      logV,
			witnessSigVs: note.VerifierList(wit1V, wit2V, wit3V),
			cps: [][]byte{
				newCP(t, "body", logS, wit1S, wit2S, wit3S),
				newCP(t, "body", logS, wit1S, wit2S, wit3S),
				newCP(t, "body", logS, wit1S, wit2S, wit3S),
				newCP(t, "body", logS, wit1S, wit2S, wit3S),
			},
			wantSigVs: []note.Verifier{logV, wit1V, wit2V, wit3V},
		}, {
			desc:         "no witness sigs",
			logSigV:      logV,
			witnessSigVs: note.VerifierList(wit1V, wit2V, wit3V),
			cps: [][]byte{
				newCP(t, "body", logS),
				newCP(t, "body", logS),
				newCP(t, "body", logS),
				newCP(t, "body", logS),
			},
			wantSigVs: []note.Verifier{logV},
		}, {
			desc:         "drop unknown witness",
			logSigV:      logV,
			witnessSigVs: note.VerifierList(wit1V, wit3V),
			cps: [][]byte{
				newCP(t, "body", logS, wit1S),
				// This one should get ignored
				newCP(t, "body", logS, wit2S),
				newCP(t, "body", logS, wit3S),
			},
			wantSigVs: []note.Verifier{logV, wit1V, wit3V},
		}, {
			desc:         "no known witness sigs",
			logSigV:      logV,
			witnessSigVs: note.VerifierList(wit2V, wit3V),
			cps: [][]byte{
				newCP(t, "body", logS, wit1S),
				newCP(t, "body", logS),
				newCP(t, "body", logS),
				newCP(t, "body", logS, wit1S),
			},
			wantSigVs: []note.Verifier{logV},
		}, {
			desc:         "combine already witnessed CP",
			logSigV:      logV,
			witnessSigVs: note.VerifierList(wit1V, wit2V, wit3V),
			cps: [][]byte{
				// This CP's already been witnessed
				newCP(t, "body", logS, wit1S, wit2S),
				// Now wit3 has signed it
				newCP(t, "body", logS, wit3S),
			},
			wantSigVs: []note.Verifier{logV, wit1V, wit2V, wit3V},
		}, {
			desc:         "no log sig on witnessed",
			logSigV:      logV,
			witnessSigVs: note.VerifierList(wit1V, wit2V, wit3V),
			cps: [][]byte{
				newCP(t, "body", wit1S, wit2S),
				newCP(t, "body", logS),
			},
			wantErr: true,
		}, {
			desc:         "no log sig on any",
			logSigV:      logV,
			witnessSigVs: note.VerifierList(wit1V, wit2V, wit3V),
			cps: [][]byte{
				newCP(t, "body", wit1S, wit2S),
				newCP(t, "body", wit3S),
			},
			wantErr: true,
		}, {
			desc:         "differing body",
			logSigV:      logV,
			witnessSigVs: note.VerifierList(wit1V, wit2V, wit3V),
			cps: [][]byte{
				newCP(t, "body", logS, wit1S, wit2S),
				newCP(t, "legs", wit3S),
			},
			wantErr: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			cp, err := Combine(test.cps, test.logSigV, test.witnessSigVs)
			gotErr := err != nil
			if test.wantErr && gotErr {
				return
			} else if gotErr != test.wantErr {
				t.Fatalf("Got err %v, want err? %t", err, test.wantErr)
			}

			n, err := note.Open(cp, note.VerifierList(test.wantSigVs...))
			if err != nil {
				t.Fatalf("Failed to re-open note: %v", err)
			}

			if l := len(n.UnverifiedSigs); l != 0 {
				for _, s := range n.UnverifiedSigs {
					t.Errorf("Unexpected sig: %v", s.Name)
				}
				t.Fatalf("Got %d unexpected signatures", l)
			}
			if got, want := len(n.Sigs), len(test.wantSigVs); got != want {
				for _, s := range n.Sigs {
					t.Errorf("Got sig: %v", s.Name)
				}
				t.Fatalf("Got %d signatures, want %d", got, want)
			}
			if n.Sigs[0].Hash != test.logSigV.KeyHash() {
				t.Fatal("First signature was not from log")
			}

		})
	}
}

func TestCombineReturnsStableOrdering(t *testing.T) {
	logS, logV := genKeyPair(t, "log")
	wit1S, wit1V := genKeyPair(t, "w1")
	wit2S, wit2V := genKeyPair(t, "w2")
	wit3S, wit3V := genKeyPair(t, "w3")
	witnessSigVs := note.VerifierList(wit1V, wit2V, wit3V)

	cps := [][]byte{
		newCP(t, "body", logS, wit1S, wit2S, wit3S),
		newCP(t, "body", logS),
	}
	cp, err := Combine(cps, logV, witnessSigVs)
	if err != nil {
		t.Fatalf("Failed to combine sigs: %v", err)
	}
	n, err := note.Open(cp, note.VerifierList(logV, wit1V, wit2V, wit3V))
	if err != nil {
		t.Fatalf("Failed to re-open note: %v", err)
	}

	if n.Sigs[0].Name != logV.Name() {
		t.Fatalf("Got signature from %q at 0, want %q", n.Sigs[0].Name, logV.Name())
	}
	// Start at index 1 since index 0 should be the log signature
	for i := 1; i < len(n.Sigs)-1; i++ {
		if ih, jh := n.Sigs[i].Hash, n.Sigs[i+1].Hash; ih > jh {
			t.Fatalf("Found out-of-order signature: index %d (%x) > index %d (%x)", i, ih, i+1, jh)
		}
	}
}

func newCP(t *testing.T, body string, sigs ...note.Signer) []byte {
	ret, err := note.Sign(&note.Note{Text: body + "\n"}, sigs...)
	if err != nil {
		t.Fatalf("Failed to sign note %q: %v", body, err)
	}
	return ret
}

func genKeyPair(t *testing.T, name string) (note.Signer, note.Verifier) {
	sKey, vKey, err := note.GenerateKey(nil, name)
	if err != nil {
		t.Fatalf("Failed to generate key %q: %v", name, err)
	}
	s, err := note.NewSigner(sKey)
	if err != nil {
		t.Fatalf("Failed to create signer %q: %v", name, err)
	}
	v, err := note.NewVerifier(vKey)
	if err != nil {
		t.Fatalf("Failed to create verifier %q: %v", name, err)
	}
	return s, v
}
