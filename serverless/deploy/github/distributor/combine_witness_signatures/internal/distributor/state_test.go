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

package distributor

import (
	"testing"

	"github.com/google/trillian-examples/formats/log"
	"golang.org/x/mod/sumdb/note"
)

func TestUpdate(t *testing.T) {
	logS, logV := genKeyPair(t, "log")
	wit1S, wit1V := genKeyPair(t, "w1")
	wit2S, wit2V := genKeyPair(t, "w2")
	wit3S, wit3V := genKeyPair(t, "w3")

	for _, test := range []struct {
		desc      string
		state     [][]byte
		opts      UpdateOpts
		incoming  [][]byte
		wantState [][]byte
		wantError bool
	}{
		{
			desc: "No update",
			opts: UpdateOpts{
				MaxWitnessSignatures: 3,
				LogSigV:              logV,
				Witnesses:            []note.Verifier{wit1V, wit2V, wit3V},
			},
			state: [][]byte{
				newCP(t, 10, logS, wit1S),
				newCP(t, 10, logS, wit1S),
				newCP(t, 8, logS, wit1S, wit2S, wit3S),
				newCP(t, 8, logS, wit1S, wit2S, wit3S),
			},
			wantState: [][]byte{
				newCP(t, 10, logS, wit1S),
				newCP(t, 10, logS, wit1S),
				newCP(t, 8, logS, wit1S, wit2S, wit3S),
				newCP(t, 8, logS, wit1S, wit2S, wit3S),
			},
		}, {
			desc: "update - no change",
			opts: UpdateOpts{
				MaxWitnessSignatures: 3,
				LogSigV:              logV,
				Witnesses:            []note.Verifier{wit1V, wit2V, wit3V},
			},
			state: [][]byte{
				newCP(t, 10, logS, wit1S),
				newCP(t, 10, logS, wit1S),
				newCP(t, 8, logS, wit1S, wit2S, wit3S),
				newCP(t, 8, logS, wit1S, wit2S, wit3S),
			},
			incoming: [][]byte{
				newCP(t, 10, logS, wit1S),
				newCP(t, 10, logS, wit1S),
			},
			wantState: [][]byte{
				newCP(t, 10, logS, wit1S),
				newCP(t, 10, logS, wit1S),
				newCP(t, 8, logS, wit1S, wit2S, wit3S),
				newCP(t, 8, logS, wit1S, wit2S, wit3S),
			},
		}, {
			desc: "update - new signature, but no change in state ordering",
			opts: UpdateOpts{
				MaxWitnessSignatures: 3,
				LogSigV:              logV,
				Witnesses:            []note.Verifier{wit1V, wit2V, wit3V},
			},
			state: [][]byte{
				newCP(t, 10, logS, wit1S),
				newCP(t, 10, logS, wit1S),
				newCP(t, 9, logS, wit1S, wit2S),
				newCP(t, 8, logS, wit1S, wit2S, wit3S),
			},
			incoming: [][]byte{
				newCP(t, 8, logS, wit3S),
			},
			wantState: [][]byte{
				newCP(t, 10, logS, wit1S),
				newCP(t, 10, logS, wit1S),
				newCP(t, 9, logS, wit1S, wit2S),
				newCP(t, 8, logS, wit1S, wit2S, wit3S),
			},
		}, {
			desc: "update - new signature makes 10 viable for next slot up",
			opts: UpdateOpts{
				MaxWitnessSignatures: 3,
				LogSigV:              logV,
				Witnesses:            []note.Verifier{wit1V, wit2V, wit3V},
			},
			state: [][]byte{
				newCP(t, 10, logS, wit1S),
				newCP(t, 10, logS, wit1S),
				newCP(t, 9, logS, wit1S, wit2S),
				newCP(t, 8, logS, wit1S, wit2S, wit3S),
			},
			incoming: [][]byte{
				newCP(t, 10, logS, wit2S),
			},
			wantState: [][]byte{
				newCP(t, 10, logS, wit1S, wit2S),
				newCP(t, 10, logS, wit1S, wit2S),
				newCP(t, 10, logS, wit1S, wit2S),
				newCP(t, 8, logS, wit1S, wit2S, wit3S),
			},
		}, {
			desc: "update - new checkpoint displaces zero slot",
			opts: UpdateOpts{
				MaxWitnessSignatures: 3,
				LogSigV:              logV,
				Witnesses:            []note.Verifier{wit1V, wit2V, wit3V},
			},
			state: [][]byte{
				newCP(t, 10, logS, wit1S),
				newCP(t, 10, logS, wit1S),
				newCP(t, 9, logS, wit1S, wit2S),
				newCP(t, 8, logS, wit1S, wit2S, wit3S),
			},
			incoming: [][]byte{
				newCP(t, 11, logS),
			},
			wantState: [][]byte{
				newCP(t, 11, logS),
				newCP(t, 10, logS, wit1S),
				newCP(t, 9, logS, wit1S, wit2S),
				newCP(t, 8, logS, wit1S, wit2S, wit3S),
			},
		}, {
			desc: "update - new checkpoint with witness sigs displaces several slots",
			opts: UpdateOpts{
				MaxWitnessSignatures: 3,
				LogSigV:              logV,
				Witnesses:            []note.Verifier{wit1V, wit2V, wit3V},
			},
			state: [][]byte{
				newCP(t, 10, logS, wit1S),
				newCP(t, 10, logS, wit1S),
				newCP(t, 9, logS, wit1S, wit2S),
				newCP(t, 8, logS, wit1S, wit2S, wit3S),
			},
			incoming: [][]byte{
				newCP(t, 11, logS, wit1S, wit2S, wit3S),
			},
			wantState: [][]byte{
				newCP(t, 11, logS, wit1S, wit2S, wit3S),
				newCP(t, 11, logS, wit1S, wit2S, wit3S),
				newCP(t, 11, logS, wit1S, wit2S, wit3S),
				newCP(t, 11, logS, wit1S, wit2S, wit3S),
			},
		}, {
			desc: "increase number of desired witness sigs",
			opts: UpdateOpts{
				MaxWitnessSignatures: 6,
				LogSigV:              logV,
				Witnesses:            []note.Verifier{wit1V, wit2V, wit3V},
			},
			state: [][]byte{
				newCP(t, 10, logS, wit1S),
				newCP(t, 10, logS, wit1S),
				newCP(t, 9, logS, wit1S, wit2S),
				newCP(t, 8, logS, wit1S, wit2S, wit3S),
			},
			incoming: [][]byte{},
			wantState: [][]byte{
				newCP(t, 10, logS, wit1S),
				newCP(t, 10, logS, wit1S),
				newCP(t, 9, logS, wit1S, wit2S),
				newCP(t, 8, logS, wit1S, wit2S, wit3S),
				nil,
				nil,
				nil,
			},
		}, {
			desc: "decrease number of desired witness sigs",
			opts: UpdateOpts{
				MaxWitnessSignatures: 1,
				LogSigV:              logV,
				Witnesses:            []note.Verifier{wit1V, wit2V, wit3V},
			},
			state: [][]byte{
				newCP(t, 10, logS, wit1S),
				newCP(t, 10, logS, wit1S),
				newCP(t, 9, logS, wit1S, wit2S),
				newCP(t, 8, logS, wit1S, wit2S, wit3S),
			},
			incoming: [][]byte{},
			wantState: [][]byte{
				newCP(t, 10, logS, wit1S),
				newCP(t, 10, logS, wit1S),
			},
		}, {
			desc: "decrease number of desired witness sigs and displace",
			opts: UpdateOpts{
				MaxWitnessSignatures: 1,
				LogSigV:              logV,
				Witnesses:            []note.Verifier{wit1V, wit2V, wit3V},
			},
			state: [][]byte{
				newCP(t, 10, logS, wit1S),
				newCP(t, 10, logS, wit1S),
				newCP(t, 9, logS, wit1S, wit2S),
				newCP(t, 8, logS, wit1S, wit2S, wit3S),
			},
			incoming: [][]byte{
				newCP(t, 12, logS, wit2S),
			},
			wantState: [][]byte{
				newCP(t, 12, logS, wit2S),
				newCP(t, 12, logS, wit2S),
			},
		}, {
			desc: "decrease number of desired witness sigs and displace with more than reqd sigs",
			opts: UpdateOpts{
				MaxWitnessSignatures: 1,
				LogSigV:              logV,
				Witnesses:            []note.Verifier{wit1V, wit2V, wit3V},
			},
			state: [][]byte{
				newCP(t, 10, logS, wit1S),
				newCP(t, 10, logS, wit1S),
				newCP(t, 9, logS, wit1S, wit2S),
				newCP(t, 8, logS, wit1S, wit2S, wit3S),
			},
			incoming: [][]byte{
				newCP(t, 12, logS, wit2S, wit3S),
			},
			wantState: [][]byte{
				newCP(t, 12, logS, wit2S, wit3S),
				newCP(t, 12, logS, wit2S, wit3S),
			},
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			got, gotErr := UpdateState(test.state, test.incoming, test.opts)
			if gotErr != nil && !test.wantError {
				t.Fatalf("Update = %v, want no error", gotErr)
			} else if gotErr == nil && test.wantError {
				t.Fatal("Update = no error, want error")
			}
			if lenGot, lenWant := len(got), len(test.wantState); lenGot != lenWant {
				t.Fatalf("Got state with %d entries, want %d", lenGot, lenWant)
			}
			witSigV := note.VerifierList(append([]note.Verifier{test.opts.LogSigV}, test.opts.Witnesses...)...)
			for i := range got {
				if gotNil, wantNil := got[i] == nil, test.wantState[i] == nil; gotNil && wantNil {
					continue
				} else if gotNil != wantNil {
					t.Fatalf("got[%d] nil? %v want nil? %v", i, gotNil, wantNil)
				}
				gotN, err := note.Open(got[i], witSigV)
				if err != nil {
					t.Logf("%s", string(got[i]))
					t.Fatalf("Failed to open got[%d]: %v", i, err)
				}
				wantN, err := note.Open(test.wantState[i], witSigV)
				if err != nil {
					t.Fatalf("Failed to open wantState[%d]: %v", i, err)
				}
				if gotN.Text != wantN.Text {
					t.Fatalf("got[%d] text != wantState:\nGOT:\n%s\nWANT:\n%s", i, gotN.Text, wantN.Text)
				}
				if gotSigs, wantSigs := len(gotN.Sigs), len(wantN.Sigs); gotSigs != wantSigs {
					t.Fatalf("got[%d] has %d sigs, want %d", i, gotSigs, wantSigs)
				}
			}
		})
	}
}

func newCP(t *testing.T, size int, sigs ...note.Signer) []byte {
	t.Helper()
	cp := log.Checkpoint{
		Origin: "testing log",
		Size:   uint64(size),
		Hash:   []byte("banana"),
	}
	ret, err := note.Sign(&note.Note{Text: string(cp.Marshal())}, sigs...)
	if err != nil {
		t.Fatalf("Failed to sign note: %v", err)
	}
	return ret
}

func genKeyPair(t *testing.T, name string) (note.Signer, note.Verifier) {
	t.Helper()
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
