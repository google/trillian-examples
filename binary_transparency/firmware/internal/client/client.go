package client

import (
	"encoding/json"
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
