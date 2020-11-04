package client_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/client"
)

func TestGetCheckpoint(t *testing.T) {
	for _, test := range []struct {
		desc    string
		body    string
		want    api.LogCheckpoint
		wantErr bool
	}{
		{
			desc: "valid 1",
			body: `{ "TreeSize": 1, "TimestampNanos": 123, "RootHash": "EjQ="}`,
			want: api.LogCheckpoint{TreeSize: 1, TimestampNanos: 123, RootHash: []byte{0x12, 0x34}},
		}, {
			desc: "valid 2",
			body: `{ "TreeSize": 10, "TimestampNanos": 1230, "RootHash": "NBI="}`,
			want: api.LogCheckpoint{TreeSize: 10, TimestampNanos: 1230, RootHash: []byte{0x34, 0x12}},
		}, {
			desc:    "garbage",
			body:    `garbage`,
			wantErr: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, api.HTTPGetRoot) {
					t.Fatalf("Got unexpected HTTP request on %q", r.URL.Path)
				}
				fmt.Fprintln(w, test.body)
			}))
			defer ts.Close()

			tsURL, err := url.Parse((ts.URL))
			if err != nil {
				t.Fatalf("Failed to parse test server URL: %v", err)
			}
			c := client.Client{LogURL: tsURL}
			cp, err := c.GetCheckpoint()
			switch {
			case err != nil && !test.wantErr:
				t.Fatalf("Got unexpected error %q", err)
			case err == nil && test.wantErr:
				t.Fatal("Got no error, but wanted error")
			case err != nil && test.wantErr:
				// expected error
			default:
				if d := cmp.Diff(*cp, test.want); len(d) != 0 {
					t.Fatalf("Got checkpoint with diff: %s", d)
				}
			}
		})
	}
}
