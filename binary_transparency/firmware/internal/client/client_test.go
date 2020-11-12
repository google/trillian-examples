package client_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/client"
)

func TestSubmitManifest(t *testing.T) {
	for _, test := range []struct {
		desc     string
		manifest []byte
		wantErr  bool
	}{
		{
			desc:     "valid",
			manifest: []byte("Boo!"),
		}, {
			desc:     "log server fails",
			manifest: []byte("Boo!"),
			wantErr:  true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check for path prefix, trimming off leading / since it's not present in the
				// const.
				// TODO Add an index to the test case to improve coverage
				if !strings.HasPrefix(r.URL.Path[1:], api.HTTPAddFirmware) {
					t.Fatalf("Got unexpected HTTP request on %q", r.URL.Path)
				}

				if test.wantErr {
					http.Error(w, "BOOM", http.StatusInternalServerError)
					return
				}

				b, err := ioutil.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("Failed to read request body: %v", err)
				}
				if diff := cmp.Diff(b, test.manifest); len(diff) != 0 {
					t.Errorf("POSTed body with unexpected diff: %v", diff)
				}
			}))
			defer ts.Close()

			tsURL, err := url.Parse((ts.URL))
			if err != nil {
				t.Fatalf("Failed to parse test server URL: %v", err)
			}
			c := client.Client{LogURL: tsURL}
			err = c.SubmitManifest(test.manifest)
			switch {
			case err != nil && !test.wantErr:
				t.Fatalf("Got unexpected error %q", err)
			case err == nil && test.wantErr:
				t.Fatal("Got no error, but wanted error")
			case err != nil && test.wantErr:
				// expected error
			default:
			}
		})
	}
}

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

func TestGetInclusion(t *testing.T) {
	cp := api.LogCheckpoint{
		TreeSize: 30,
	}
	for _, test := range []struct {
		desc    string
		body    string
		want    api.InclusionProof
		wantErr bool
	}{
		{
			desc: "valid 1",
			body: `{ "LeafIndex": 2, "Proof": ["qg==", "uw==", "zA=="]}`,
			want: api.InclusionProof{LeafIndex: 2, Proof: [][]byte{{0xAA}, {0xBB}, {0xCC}}},
		}, {
			desc: "valid 2",
			body: `{ "LeafIndex": 20, "Proof": ["3Q==", "7g=="]}`,
			want: api.InclusionProof{LeafIndex: 20, Proof: [][]byte{{0xDD}, {0xEE}}},
		}, {
			desc:    "garbage",
			body:    `garbage`,
			wantErr: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check for path prefix, trimming off leading / since it's not present in the
				// const.
				// TODO Add an index to the test case to improve coverage
				if !strings.HasPrefix(r.URL.Path[1:], api.HTTPGetInclusion) {
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
			ip, err := c.GetInclusion([]byte{}, cp)
			switch {
			case err != nil && !test.wantErr:
				t.Fatalf("Got unexpected error %q", err)
			case err == nil && test.wantErr:
				t.Fatal("Got no error, but wanted error")
			case err != nil && test.wantErr:
				// expected error
			default:
				if d := cmp.Diff(ip, test.want); len(d) != 0 {
					t.Fatalf("Got checkpoint with diff: %s", d)
				}
			}
		})
	}
}

func TestGetManifestAndProof(t *testing.T) {
	for _, test := range []struct {
		desc    string
		body    string
		want    api.InclusionProof
		wantErr bool
	}{
		{
			desc: "valid 1",
			body: `{ "Value":"EjQ=", "Proof": ["qg==", "uw==", "zA=="]}`,
			want: api.InclusionProof{Value: []byte{0x12, 0x34}, Proof: [][]byte{{0xAA}, {0xBB}, {0xCC}}},
		}, {
			desc: "valid 2",
			body: `{ "Value":"NBI=","Proof": ["3Q==", "7g=="]}`,
			want: api.InclusionProof{Value: []byte{0x34, 0x12}, Proof: [][]byte{{0xDD}, {0xEE}}},
		}, {
			desc:    "garbage",
			body:    `garbage`,
			wantErr: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check for path prefix, trimming off leading / since it's not present in the
				// const.
				// TODO Add an index to the test case to improve coverage
				if !strings.HasPrefix(r.URL.Path[1:], api.HTTPGetManifestEntryAndProof) {
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
			ip, err := c.GetManifestEntryAndProof(api.GetFirmwareManifestRequest{0, 0})
			switch {
			case err != nil && !test.wantErr:
				t.Fatalf("Got unexpected error %q", err)
			case err == nil && test.wantErr:
				t.Fatal("Got no error, but wanted error")
			case err != nil && test.wantErr:
				// expected error
			default:
				if d := cmp.Diff(*ip, test.want); len(d) != 0 {
					t.Fatalf("Got response with diff: %s", d)
				}
			}
		})
	}
}

func TestGetConsistency(t *testing.T) {
	for _, test := range []struct {
		desc    string
		body    string
		From    uint64
		To      uint64
		want    api.ConsistencyProof
		wantErr bool
	}{
		{
			desc: "valid 1",
			body: `{"Proof": ["qg==", "uw==", "zA=="]}`,
			From: 0,
			To:   1,
			want: api.ConsistencyProof{Proof: [][]byte{{0xAA}, {0xBB}, {0xCC}}},
		}, {
			desc: "valid 2",
			body: `{"Proof": ["3Q==", "7g=="]}`,
			From: 1,
			To:   2,
			want: api.ConsistencyProof{Proof: [][]byte{{0xDD}, {0xEE}}},
		}, {
			desc:    "garbage",
			body:    `garbage`,
			wantErr: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check for path prefix, trimming off leading / since it's not present in the
				// const.
				// TODO Add an index to the test case to improve coverage
				if !strings.HasPrefix(r.URL.Path[1:], api.HTTPGetConsistency) {
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
			cp, err := c.GetConsistencyProof(api.GetConsistencyRequest{test.From, test.To})
			switch {
			case err != nil && !test.wantErr:
				t.Fatalf("Got unexpected error %q", err)
			case err == nil && test.wantErr:
				t.Fatal("Got no error, but wanted error")
			case err != nil && test.wantErr:
				// expected error
			default:
				if d := cmp.Diff(*cp, test.want); len(d) != 0 {
					t.Fatalf("Got response with diff: %s", d)
				}
			}
		})
	}
}
