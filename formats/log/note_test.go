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
	"fmt"
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
	logVerifier, err := note.NewVerifier(logVK)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	known1Verifier, err := note.NewVerifier(known1VK)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	known2Verifier, err := note.NewVerifier(known2VK)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	cp := log.Checkpoint{
		Origin: "TestParseCheckpoint",
		Size:   42,
		Hash:   []byte("abcdef"),
	}
	noteBody := string(cp.Marshal())

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
		desc     string
		logID    string
		noteBody string
		sigs     []note.Signer
		wantErr  bool
		wantSigs int
	}{
		{
			desc:     "no sigs",
			logID:    "TestParseCheckpoint",
			noteBody: noteBody,
			sigs:     []note.Signer{},
			wantErr:  true,
			wantSigs: 0,
		},
		{
			desc:     "just log sig",
			logID:    "TestParseCheckpoint",
			noteBody: noteBody,
			sigs:     []note.Signer{lns},
			wantErr:  false,
			wantSigs: 1,
		},
		{
			desc:     "bad body good sigs",
			logID:    "TestParseCheckpoint",
			noteBody: "if this is a valid checkpoint then I'll eat my hat\n",
			sigs:     []note.Signer{lns},
			wantErr:  true,
			wantSigs: 1,
		},
		{
			desc:     "log and known1 sig",
			logID:    "TestParseCheckpoint",
			noteBody: noteBody,
			sigs:     []note.Signer{lns, k1ns},
			wantErr:  false,
			wantSigs: 2,
		},
		{
			desc:     "log and known2 sig",
			logID:    "TestParseCheckpoint",
			noteBody: noteBody,
			sigs:     []note.Signer{lns, k1ns},
			wantErr:  false,
			wantSigs: 2,
		},
		{
			desc:     "log, known1, and unknown sig",
			logID:    "TestParseCheckpoint",
			noteBody: noteBody,
			sigs:     []note.Signer{lns, k1ns, uns},
			wantErr:  false,
			wantSigs: 2,
		},
		{
			desc:     "just required sigs",
			logID:    "TestParseCheckpoint",
			noteBody: noteBody,
			sigs:     []note.Signer{lns, k1ns, k2ns},
			wantErr:  false,
			wantSigs: 3,
		},
		{
			desc:     "one verifier signs twice",
			logID:    "TestParseCheckpoint",
			noteBody: noteBody,
			sigs:     []note.Signer{lns, k1ns, k1ns},
			wantErr:  false,
			wantSigs: 2,
		},
		{
			desc:     "just required sigs in mixed order",
			logID:    "TestParseCheckpoint",
			noteBody: noteBody,
			sigs:     []note.Signer{k2ns, lns, k1ns},
			wantErr:  false,
			wantSigs: 3,
		}, {
			desc:     "all sigs",
			logID:    "TestParseCheckpoint",
			noteBody: noteBody,
			sigs:     []note.Signer{lns, k1ns, k2ns, uns},
			wantErr:  false,
			wantSigs: 3,
		}, {
			desc:     "just known",
			logID:    "TestParseCheckpoint",
			noteBody: noteBody,
			sigs:     []note.Signer{k1ns, k2ns},
			wantErr:  true,
			wantSigs: 2,
		}, {
			desc:     "all sigs but wrong logID",
			logID:    "this is not the logID you are looking for",
			noteBody: noteBody,
			sigs:     []note.Signer{lns, k1ns, k2ns, uns},
			wantErr:  true,
			wantSigs: 3,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			nBs, err := note.Sign(&note.Note{Text: test.noteBody}, test.sigs...)
			if err != nil {
				t.Fatalf("Failed to sign note: %v", err)
			}

			// Now parse what we have created.
			_, _, n, err := log.ParseCheckpoint(nBs, test.logID, logVerifier, known1Verifier, known2Verifier)
			if gotErr := err != nil; gotErr != test.wantErr {
				t.Errorf("gotErr %t != wantErr %t (%v)", gotErr, test.wantErr, err)
			}
			gotSigs := 0
			if n != nil {
				gotSigs = len(n.Sigs)
			}
			if gotSigs != test.wantSigs {
				t.Errorf("got %d signatures but wanted %d", gotSigs, test.wantSigs)
			}
		})
	}
}

func TestSumDBNoteParsing(t *testing.T) {
	logVerifier, err := note.NewVerifier("sum.golang.org+033de0ae+Ac4zctda0e5eza+HJyk9SxEdh+s3Ux18htTTAD8OuAn8")
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	noteString := `go.sum database tree
6476701
mb8QLQIs0Z0yP5Cstq6guj87oXWeC9gEM8oVikmm9Wk=

— sum.golang.org Az3grsLX85Gz+s1SiTbgkuqxgItqFq7gsMUEyVnrsa9LM7Us9S+1xbFIGu95949rj4nPRYfvimWEPWL+o3GeoWwOoAw=
`

	for _, test := range []struct {
		desc    string
		logID   string
		wantErr bool
	}{
		{
			desc:    "hunky dory",
			logID:   "go.sum database tree",
			wantErr: false,
		},
		{
			desc:    "wrong logID",
			logID:   "Go rocks but I might have the wrong ID",
			wantErr: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			cp, _, _, err := log.ParseCheckpoint([]byte(noteString), test.logID, logVerifier)
			if err != nil {
				if !test.wantErr {
					t.Fatalf("Failed to parse checkpoint note: %v", err)
				}
				if cp != nil {
					t.Errorf("Expected empty checkpoint because of error but got %+v", cp)
				}
				// Always return after error because remaining checks are for cp, which is nil.
				return
			}
			if test.wantErr {
				t.Fatal("Expected error but didn't get it")
			}
			if got, want := cp.Size, uint64(6476701); got != want {
				t.Errorf("expected size %d but got %d", want, got)
			}
			if got, want := cp.Origin, "go.sum database tree"; got != want {
				t.Errorf("expected log ID %q but got %q", want, got)
			}
		})
	}
}

func BenchmarkParse(b *testing.B) {
	logVerifier, err := note.NewVerifier("sum.golang.org+033de0ae+Ac4zctda0e5eza+HJyk9SxEdh+s3Ux18htTTAD8OuAn8")
	if err != nil {
		b.Fatalf("NewVerifier: %v", err)
	}
	baseNote := `go.sum database tree
6476701
mb8QLQIs0Z0yP5Cstq6guj87oXWeC9gEM8oVikmm9Wk=

— sum.golang.org Az3grsLX85Gz+s1SiTbgkuqxgItqFq7gsMUEyVnrsa9LM7Us9S+1xbFIGu95949rj4nPRYfvimWEPWL+o3GeoWwOoAw=
`
	for _, test := range []struct {
		desc      string
		verifiers []string
		sigs      []string
	}{
		{
			desc: "bare minimum",
		},
		{
			desc: "one verifier",
			verifiers: []string{
				"monkeys+db4d9f7e+AULaJMvTtDLHPUcUrjdDad9vDlh/PTfC2VV60JUtCfWT",
			},
			sigs: []string{
				"— monkeys 202ffh9ve46Mqbhuyb4wnC2Pwdan5EsXQym/tF1ggXJqhjGosU4T5intH4iKMRgDLxrnXuIjRaU2NbvAnaepM6Y+8ww=",
			},
		},
		{
			desc: "many verifiers",
			verifiers: []string{
				"monkeys+db4d9f7e+AULaJMvTtDLHPUcUrjdDad9vDlh/PTfC2VV60JUtCfWT",
				"bananas+cf639f13+AaPjhFnPCQnid/Ql32KWhmh+uk72FVRfK+2DLmO3BI3M",
				"witness+f13a86db+AdYV1Ztajd9BvyjP2HgpwrqYL6TjOwIjGMOq8Bu42xbN",
			},
			sigs: []string{
				"— monkeys 202ffh9ve46Mqbhuyb4wnC2Pwdan5EsXQym/tF1ggXJqhjGosU4T5intH4iKMRgDLxrnXuIjRaU2NbvAnaepM6Y+8ww=",
				"— bananas z2OfE+2bbsBZ7NLgfuslc0v7NFW2QyexJ7fhePpoL2N/P7N/COd5S+JNHbQEAlYzz2C5sg0E+x/MggS1mDR6t3/9rQw=",
				"— witness 8TqG25yWGFLdT7WPf/WoyeG3LwqtkXP+8S3yykOVm6EMh4hPgCke6eSVPU4BdtXNMPJBXKF+UpDbGgCdcpR8SVSoqAU=",
			},
		},
		{
			desc: "many sigs, 1 verifier",
			verifiers: []string{
				"monkeys+db4d9f7e+AULaJMvTtDLHPUcUrjdDad9vDlh/PTfC2VV60JUtCfWT",
			},
			sigs: []string{
				"— monkeys 202ffh9ve46Mqbhuyb4wnC2Pwdan5EsXQym/tF1ggXJqhjGosU4T5intH4iKMRgDLxrnXuIjRaU2NbvAnaepM6Y+8ww=",
				"— bananas z2OfE+2bbsBZ7NLgfuslc0v7NFW2QyexJ7fhePpoL2N/P7N/COd5S+JNHbQEAlYzz2C5sg0E+x/MggS1mDR6t3/9rQw=",
				"— witness 8TqG25yWGFLdT7WPf/WoyeG3LwqtkXP+8S3yykOVm6EMh4hPgCke6eSVPU4BdtXNMPJBXKF+UpDbGgCdcpR8SVSoqAU=",
				"— random1 8TqG25yWGFLdT7WPf/WoyeG3LwqtkXP+8S3yykOVm6EMh4hPgCke6eSVPU4BdtXNMPJBXKF+UpDbGgCdcpR8SVSoqAU=",
				"— random2 8TqG25yWGFLdT7WPf/WoyeG3LwqtkXP+8S3yykOVm6EMh4hPgCke6eSVPU4BdtXNMPJBXKF+UpDbGgCdcpR8SVSoqAU=",
			},
		},
	} {
		b.Run(test.desc, func(b *testing.B) {
			vs := make([]note.Verifier, len(test.verifiers))
			for i, v := range test.verifiers {
				vs[i], err = note.NewVerifier(v)
				if err != nil {
					if err != nil {
						b.Fatalf("NewVerifier: %v", err)
					}
				}
			}
			note := baseNote
			for _, s := range test.sigs {
				note = fmt.Sprintf("%s%s\n", note, s)
			}
			for n := 0; n < b.N; n++ {
				if _, _, _, err := log.ParseCheckpoint([]byte(note), "go.sum database tree", logVerifier, vs...); err != nil {
					b.Fatalf("Failed to parse: %v", err)
				}
			}
		})
	}
}
