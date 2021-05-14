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

package format_test

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian-examples/format"
)

func TestMarshal(t *testing.T) {
	for _, test := range []struct {
		c    format.Checkpoint
		want string
	}{
		{
			c: format.Checkpoint{
				Ecosystem: "Log Checkpoint v0",
				Size:      123,
				RootHash:  []byte("bananas"),
			},
			want: "Log Checkpoint v0\n123\nYmFuYW5hcw==\n",
		}, {
			c: format.Checkpoint{
				Ecosystem: "Banana Checkpoint v5",
				Size:      9944,
				RootHash:  []byte("the view from the tree tops is great!"),
			},
			want: "Banana Checkpoint v5\n9944\ndGhlIHZpZXcgZnJvbSB0aGUgdHJlZSB0b3BzIGlzIGdyZWF0IQ==\n",
		},
	} {
		t.Run(string(test.c.RootHash), func(t *testing.T) {
			got := test.c.Marshal()
			if string(got) != test.want {
				t.Fatalf("Marshal = %q, want %q", got, test.want)
			}
		})
	}
}

func TestUnmarshalLogState(t *testing.T) {
	for _, test := range []struct {
		desc     string
		m        string
		want     format.Checkpoint
		wantRest []byte
		wantErr  bool
	}{
		{
			desc: "valid one",
			m:    "Log Checkpoint v0\n123\nYmFuYW5hcw==\n",
			want: format.Checkpoint{
				Ecosystem: "Log Checkpoint v0",
				Size:      123,
				RootHash:  []byte("bananas"),
			},
		}, {
			desc: "valid with different ecosystem",
			m:    "Banana Checkpoint v1\n9944\ndGhlIHZpZXcgZnJvbSB0aGUgdHJlZSB0b3BzIGlzIGdyZWF0IQ==\n",
			want: format.Checkpoint{
				Ecosystem: "Banana Checkpoint v1",
				Size:      9944,
				RootHash:  []byte("the view from the tree tops is great!"),
			},
		}, {
			desc: "valid with trailing data",
			m:    "Log Checkpoint v0\n9944\ndGhlIHZpZXcgZnJvbSB0aGUgdHJlZSB0b3BzIGlzIGdyZWF0IQ==\nHere's some associated data.\n",
			want: format.Checkpoint{
				Ecosystem: "Log Checkpoint v0",
				Size:      9944,
				RootHash:  []byte("the view from the tree tops is great!"),
			},
			wantRest: []byte("Here's some associated data.\n"),
		}, {
			desc: "valid with multiple trailing data lines",
			m:    "Log Checkpoint v0\n9944\ndGhlIHZpZXcgZnJvbSB0aGUgdHJlZSB0b3BzIGlzIGdyZWF0IQ==\nlots\nof\nlines\n",
			want: format.Checkpoint{
				Ecosystem: "Log Checkpoint v0",
				Size:      9944,
				RootHash:  []byte("the view from the tree tops is great!"),
			},
			wantRest: []byte("lots\nof\nlines\n"),
		}, {
			desc: "valid with trailing newlines",
			m:    "Log Checkpoint v0\n9944\ndGhlIHZpZXcgZnJvbSB0aGUgdHJlZSB0b3BzIGlzIGdyZWF0IQ==\n\n\n\n",
			want: format.Checkpoint{
				Ecosystem: "Log Checkpoint v0",
				Size:      9944,
				RootHash:  []byte("the view from the tree tops is great!"),
			},
			wantRest: []byte("\n\n\n"),
		}, {
			desc:    "invalid empty header",
			m:       "\n9944\ndGhlIHZpZXcgZnJvbSB0aGUgdHJlZSB0b3BzIGlzIGdyZWF0IQ==\n",
			wantErr: true,
		}, {
			desc:    "invalid size - not a number",
			m:       "Log Checkpoint v0\nbananas\ndGhlIHZpZXcgZnJvbSB0aGUgdHJlZSB0b3BzIGlzIGdyZWF0IQ==\n",
			wantErr: true,
		}, {
			desc:    "invalid size - negative",
			m:       "Log Checkpoint v0\n-34\ndGhlIHZpZXcgZnJvbSB0aGUgdHJlZSB0b3BzIGlzIGdyZWF0IQ==\n",
			wantErr: true,
		}, {
			desc:    "invalid size - too large",
			m:       "Log Checkpoint v0\n3438945738945739845734895735\ndGhlIHZpZXcgZnJvbSB0aGUgdHJlZSB0b3BzIGlzIGdyZWF0IQ==\n",
			wantErr: true,
		}, {
			desc:    "invalid roothash - not base64",
			m:       "Log Checkpoint v0\n123\nThisIsn'tBase64\n",
			wantErr: true,
		},
	} {
		t.Run(string(test.desc), func(t *testing.T) {
			var got format.Checkpoint
			var gotErr error
			var gotRest []byte
			if gotRest, gotErr = got.Unmarshal([]byte(test.m)); (gotErr != nil) != test.wantErr {
				t.Fatalf("Unmarshal = %q, wantErr: %T", gotErr, test.wantErr)
			}
			if diff := cmp.Diff(test.want, got); len(diff) != 0 {
				t.Fatalf("Unmarshalled Checkpoint with diff %s", diff)
			}
			if !bytes.Equal(test.wantRest, gotRest) {
				t.Fatalf("got rest %x, want %x", gotRest, test.wantRest)
			}
		})
	}
}

////////////////////////////////////////////////////////////////////////////////
// Below is an example of embedding the minimal checkpoint as one way to extend
// it to include additional ecosystem-specific data.
// Reimplementing parsing of the full extended structure would be fine too.

// moonLogCheckpoint is a hypothetical checkpoint for an ecosystem which requires
// its checkpoints to commit to more data than the minimum common checkpoint does.
type moonLogCheckpoint struct {
	format.Checkpoint
	Timestamp uint64
	Phase     string
}

// Unmarshal knows how to unmarshal the moon log data.
// It delegates to the embedded Checkpoint to unmarshal itself first, before
// attempting to unmarshal the Moon ecosystem specific data.
func (m *moonLogCheckpoint) Unmarshal(data []byte) error {
	const delim = "\n"
	rest, err := m.Checkpoint.Unmarshal(data)
	if err != nil {
		return err
	}
	l := strings.Split(strings.TrimRight(string(rest), delim), delim)
	if el := len(l); el != 2 {
		return fmt.Errorf("want 2 lines of other data, got %d", el)
	}
	ts, err := strconv.ParseUint(l[0], 16, 64)
	if err != nil {
		return fmt.Errorf("failed to parse timestamp: %w", err)
	}
	m.Timestamp, m.Phase = ts, l[1]
	return nil
}

func TestExtendCheckpoint(t *testing.T) {
	const raw = "Moon Log Checkpoint v0\n4027504\naXQncyBhIHJvb3QgaGFzaA==\n6086d1a9\nWaxing gibbous\n"
	want := moonLogCheckpoint{
		Checkpoint: format.Checkpoint{
			Ecosystem: "Moon Log Checkpoint v0",
			Size:      4027504,
			RootHash:  []byte("it's a root hash"),
		},
		Timestamp: 0x6086d1a9,
		Phase:     "Waxing gibbous",
	}

	var got moonLogCheckpoint
	if err := got.Unmarshal([]byte(raw)); err != nil {
		t.Fatalf("Unmarshal = %q", err)
	}
	if diff := cmp.Diff(got, want); len(diff) != 0 {
		t.Fatalf("Unmarshal = diff %s", diff)
	}
}
