package http

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
)

func TestGetWitnessCheckpoint(t *testing.T) {
	for _, test := range []struct {
		desc     string
		cp       api.LogCheckpoint
		wantBody string
	}{
		{
			desc:     "Witness Checkpoint retrieval",
			cp:       api.LogCheckpoint{TreeSize: 1, TimestampNanos: 123, RootHash: []byte{0x12, 0x34}},
			wantBody: `{"TreeSize":1,"RootHash":"EjQ=","TimestampNanos":123}`,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {

			witness := NewWitness(FakeStore{test.cp}, " ", 5)

			ts := httptest.NewServer(http.HandlerFunc(witness.getCheckpoint))
			defer ts.Close()

			client := ts.Client()
			resp, err := client.Get(ts.URL)
			if err != nil {
				t.Errorf("error response: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("status code not OK: %v", resp.StatusCode)
			}
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Errorf("failed to read body: %v", err)
			}
			if string(body) != test.wantBody {
				t.Errorf("got '%s' want '%s'", string(body), test.wantBody)
			}
		})
	}
}

type FakeStore struct {
	scp api.LogCheckpoint
}

func (f FakeStore) StoreCP(wcp api.LogCheckpoint) error {
	f.scp = wcp
	return nil
}

func (f FakeStore) RetrieveCP() (api.LogCheckpoint, error) {
	return f.scp, nil
}
