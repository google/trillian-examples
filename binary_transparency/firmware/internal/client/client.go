package client

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
)

// Client is an HTTP client for the FT personality.
type Client struct {
	// LogURL is the base URL for the FT log.
	LogURL *url.URL
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
	u, err := c.LogURL.Parse(fmt.Sprintf("%s/for-leaf-hash/%s/in-tree-of/%d", api.HTTPGetInclusion, base64.StdEncoding.EncodeToString(hash), cp.TreeSize))
	if err != nil {
		return api.InclusionProof{}, err
	}
	r, err := http.Get(u.String())
	if err != nil {
		return api.InclusionProof{}, err
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

	var cp api.ConsistencyProof
	if err := json.NewDecoder(r.Body).Decode(&cp); err != nil {
		return nil, err
	}

	return &cp, nil
}
