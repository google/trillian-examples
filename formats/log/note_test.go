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

package log_test

import (
	"testing"

	"github.com/google/trillian-examples/formats/log"
	"golang.org/x/mod/sumdb/note"
)

var (
	logVK = "Log+2271c621+AemuH5ooBpEPcr+W8onrjA1NfuzBVHdezakU81Ekarvs"
	logSK = "PRIVATE+KEY+Log+2271c621+AdEIq9FLRQni54Wg68T96VJO+iayOaulswav2DUMKgvQ"

	known1VK = "Known+451c786d+AXAQ0T7lbwuY4ABJ9/MYY9bWV3hmSyOGVt2x42NkmdUH"
	known1SK = "PRIVATE+KEY+Known+451c786d+AQPRoE2+Z+Ed1HaHBA9F/FfracvcZ2rDU39kCecMEoci"

	known2VK = "KnownAgain+4893b26c+AQ+LR7BFF/F/dkpIBjBpSvT0crkteizuUkavAoGeaYMj"
	known2SK = "PRIVATE+KEY+KnownAgain+4893b26c+ARhZda/l25zjGZswYLDZsLNRzNpmv5wYTN9IoTr1OJ2+"

	unknownVK = "Unknown+fdbb2e08+AWgRIYZ+8x1Vl+1Q+sR8zciNHiYI1SdlBjNw+RV0rots"
	unknownSK = "PRIVATE+KEY+Unknown+fdbb2e08+Aav2knTC6orbhXX8pMWuNiWgO+Wwk2DB+h1eU2Q1W1nU"

	_ = unknownVK
)

func TestParseCheckpoint(t *testing.T) {
	cp := log.Checkpoint{
		Ecosystem: "TestParseCheckpoint",
		Size:      42,
		Hash:      []byte("abcdef"),
	}
	noteBody := cp.Marshal()

	lns, err := note.NewSigner(logSK)
	if err != nil {
		t.Fatalf("couldn't create log signer: %v", err)
	}
	k1ns, err := note.NewSigner(known1SK)
	if err != nil {
		t.Fatalf("couldn't create known signer: %v", err)
	}
	k2ns, err := note.NewSigner(known2SK)
	if err != nil {
		t.Fatalf("couldn't create known signer: %v", err)
	}
	uns, err := note.NewSigner(unknownSK)
	if err != nil {
		t.Fatalf("couldn't create unknown signer: %v", err)
	}

	for _, test := range []struct {
		desc    string
		sigs    []note.Signer
		wantErr bool
	}{
		{
			desc:    "no sigs",
			sigs:    []note.Signer{},
			wantErr: true,
		},
		{
			desc:    "just log sig",
			sigs:    []note.Signer{lns},
			wantErr: true,
		},
		{
			desc:    "log and known1 sig",
			sigs:    []note.Signer{lns, k1ns},
			wantErr: true,
		},
		{
			desc:    "log and known2 sig",
			sigs:    []note.Signer{lns, k1ns},
			wantErr: true,
		},
		{
			desc:    "log, known1, and unknown sig",
			sigs:    []note.Signer{lns, k1ns, uns},
			wantErr: true,
		},
		{
			desc:    "just required sigs",
			sigs:    []note.Signer{lns, k1ns, k2ns},
			wantErr: false,
		},
		{
			desc:    "one verifier signs twice",
			sigs:    []note.Signer{lns, k1ns, k1ns},
			wantErr: true,
		},
		{
			desc:    "just required sigs in mixed order",
			sigs:    []note.Signer{k2ns, lns, k1ns},
			wantErr: false,
		}, {
			desc:    "all sigs",
			sigs:    []note.Signer{lns, k1ns, k2ns, uns},
			wantErr: false,
		}, {
			desc:    "just known",
			sigs:    []note.Signer{k1ns, k2ns},
			wantErr: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			parser, err := log.NewCheckpointParser(logVK, known1VK, known2VK)
			if err != nil {
				t.Fatalf("NewCheckpointParser: %v", err)
			}
			n, err := note.Sign(&note.Note{Text: string(noteBody)}, test.sigs...)
			if err != nil {
				t.Fatalf("Failed to sign note: %v", err)
			}

			// Now parse what we have created.
			cp, err := parser.Parse(n)
			if gotErr := err != nil; gotErr != test.wantErr {
				t.Fatalf("gotErr %t != wantErr %t (%v)", gotErr, test.wantErr, err)
			}
			_ = cp
		})
	}
}