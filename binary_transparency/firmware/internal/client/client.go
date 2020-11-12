package client

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/golang/glog"
	"google.golang.org/grpc/status"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
)

// Client is an HTTP client for the FT personality.
//
// TODO(al): split this into Client and SubmitClient.
type Client struct {
	// LogURL is the base URL for the FT log.
	LogURL *url.URL
}

// SubmitManifest sends a firmware manifest file to the log server.
func (c Client) SubmitManifest(manifest []byte) error {
	u, err := c.LogURL.Parse(api.HTTPAddFirmware)
	if err != nil {
		return err
	}
	glog.V(1).Infof("Submitting to %v", u.String())
	r, err := http.Post(u.String(), "application/json", bytes.NewBuffer(manifest))
	if err != nil {
		return fmt.Errorf("failed to publish to log endpoint (%s): %w", u, err)
	}
	if r.StatusCode != http.StatusOK {
		return errFromResponse("failed to submit to log", r)
	}
	return nil
}

// GetCheckpoint returns a new LogCheckPoint from the server.
func (c Client) GetCheckpoint() (*api.LogCheckpoint, error) {
	u, err := c.LogURL.Parse(api.HTTPGetRoot)
	if err != nil {
		return nil, err
	}
	r, err := http.Get(u.String())
	if err != nil {
		return nil, err
	}
	if r.StatusCode != 200 {
		return &api.LogCheckpoint{}, errFromResponse("failed to fetch checkpoint", r)
	}

	var cp api.LogCheckpoint
	if err := json.NewDecoder(r.Body).Decode(&cp); err != nil {
		return nil, err
	}
	// TODO(al): Check signature
	return &cp, nil
}

// GetInclusion returns an inclusion proof for the statement under the given checkpoint.
func (c Client) GetInclusion(statement []byte, cp api.LogCheckpoint) (api.InclusionProof, error) {
	hash := HashLeaf(statement)
	u, err := c.LogURL.Parse(fmt.Sprintf("%s/for-leaf-hash/%s/in-tree-of/%d", api.HTTPGetInclusion, base64.URLEncoding.EncodeToString(hash), cp.TreeSize))
	if err != nil {
		return api.InclusionProof{}, err
	}
	glog.V(2).Infof("Fetching inclusion proof from %q", u.String())
	r, err := http.Get(u.String())
	if err != nil {
		return api.InclusionProof{}, err
	}
	if r.StatusCode != 200 {
		return api.InclusionProof{}, errFromResponse("failed to fetch inclusion proof", r)
	}

	var ip api.InclusionProof
	err = json.NewDecoder(r.Body).Decode(&ip)
	return ip, err
}

// GetManifestEntryAndProof returns the manifest and proof from the server, for given Index and TreeSize
func (c Client) GetManifestEntryAndProof(request api.GetFirmwareManifestRequest) (*api.InclusionProof, error) {
	url := fmt.Sprintf("%s/at/%d/in-tree-of/%d", api.HTTPGetManifestEntryAndProof, request.Index, request.TreeSize)

	u, err := c.LogURL.Parse(url)
	if err != nil {
		return nil, err
	}

	r, err := http.Get(u.String())
	if err != nil {
		return nil, err
	}
	if r.StatusCode != 200 {
		return nil, errFromResponse("failed to fetch entry and proof", r)
	}

	var mr api.InclusionProof
	if err := json.NewDecoder(r.Body).Decode(&mr); err != nil {
		return nil, err
	}

	return &mr, nil
}

// GetConsistencyProof returns the Consistency Proof from the server, for the two given snapshots
func (c Client) GetConsistencyProof(request api.GetConsistencyRequest) (*api.ConsistencyProof, error) {
	url := fmt.Sprintf("%s/from/%d/to/%d", api.HTTPGetConsistency, request.From, request.To)

	u, err := c.LogURL.Parse(url)
	if err != nil {
		return nil, err
	}

	r, err := http.Get(u.String())
	if err != nil {
		return nil, err
	}
	if r.StatusCode != 200 {
		return nil, errFromResponse("failed to fetch consistency proof", r)
	}

	var cp api.ConsistencyProof
	if err := json.NewDecoder(r.Body).Decode(&cp); err != nil {
		return nil, err
	}

	return &cp, nil
}

func errFromResponse(m string, r *http.Response) error {
	if r.StatusCode == 200 {
		return nil
	}

	b, _ := ioutil.ReadAll(r.Body) // Ignore any error, we want to ensure we return the right status code which we already know.

	msg := fmt.Sprintf("%s: %s", m, string(b))
	return status.New(codeFromHTTPResponse(r.StatusCode), msg).Err()
}
