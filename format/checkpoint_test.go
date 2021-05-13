package format

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestUnmarshalLogState(t *testing.T) {
	for _, test := range []struct {
		desc    string
		m       string
		want    Checkpoint
		wantErr bool
	}{
		{
			desc: "valid one",
			m:    "Log Checkpoint v0\n123\nYmFuYW5hcw==\n",
			want: Checkpoint{
				Ecosystem: "Log Checkpoint v0",
				Size:      123,
				RootHash:  []byte("bananas"),
			},
		}, {
			desc: "valid with different ecosystem",
			m:    "Banana Checkpoint v1\n9944\ndGhlIHZpZXcgZnJvbSB0aGUgdHJlZSB0b3BzIGlzIGdyZWF0IQ==\n",
			want: Checkpoint{
				Ecosystem: "Banana Checkpoint v1",
				Size:      9944,
				RootHash:  []byte("the view from the tree tops is great!"),
			},
		}, {
			desc: "valid with trailing data",
			m:    "Log Checkpoint v0\n9944\ndGhlIHZpZXcgZnJvbSB0aGUgdHJlZSB0b3BzIGlzIGdyZWF0IQ==\nHere's some associated data.",
			want: Checkpoint{
				Ecosystem: "Log Checkpoint v0",
				Size:      9944,
				RootHash:  []byte("the view from the tree tops is great!"),
			},
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
			var got Checkpoint
			if gotErr := got.Unmarshal([]byte(test.m)); (gotErr != nil) != test.wantErr {
				t.Fatalf("Unmarshal = %q, wantErr: %T", gotErr, test.wantErr)
			}
			if diff := cmp.Diff(test.want, got); len(diff) != 0 {
				t.Fatalf("Unmarshal = diff %s", diff)
			}
		})
	}
}
