package api_test

import (
	"testing"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
)

func TestLogCheckpointString(t *testing.T) {
	for _, test := range []struct {
		cp   api.LogCheckpoint
		want string
	}{
		{
			cp:   api.LogCheckpoint{},
			want: "{size 0 @ 0 root: 0x}",
		}, {
			cp:   api.LogCheckpoint{TreeSize: 10, TimestampNanos: 234, RootHash: []byte{0x12, 0x34, 0x56}},
			want: "{size 10 @ 234 root: 0x123456}",
		},
	} {
		if got, want := test.cp.String(), test.want; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}
