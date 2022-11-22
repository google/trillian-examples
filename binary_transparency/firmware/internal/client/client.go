// Copyright 2020 Google LLC. All Rights Reserved.
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

package client

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/transparency-dev/merkle/rfc6962"
	"golang.org/x/mod/sumdb/note"
	"google.golang.org/grpc/status"
)

// ReadonlyClient is an HTTP client for the FT personality.
//
// TODO(al): split this into Client and SubmitClient.
type ReadonlyClient struct {
	// LogURL is the base URL for the FT log.
	LogURL *url.URL

	LogSigVerifier note.Verifier
}

// SubmitClient extends ReadonlyClient to also know how to submit entries
type SubmitClient struct {
	*ReadonlyClient
}

// PublishFirmware sends a firmware manifest and corresponding image to the log server.
func (c SubmitClient) PublishFirmware(manifest, image []byte) error {
	u, err := c.LogURL.Parse(api.HTTPAddFirmware)
	if err != nil {
		return err
	}
	glog.V(1).Infof("Submitting to %v", u.String())
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	// Write the manifest JSON part
	mh := make(textproto.MIMEHeader)
	mh.Set("Content-Type", "application/json")
	partWriter, err := w.CreatePart(mh)
	if err != nil {
		return err
	}
	if _, err := io.Copy(partWriter, bytes.NewReader(manifest)); err != nil {
		return err
	}

	// Write the binary FW image part
	mh = make(textproto.MIMEHeader)
	mh.Set("Content-Type", "application/octet-stream")
	partWriter, err = w.CreatePart(mh)
	if err != nil {
		return err
	}
	if _, err := io.Copy(partWriter, bytes.NewReader(image)); err != nil {
		return err
	}

	// Finish off the multipart request
	if err := w.Close(); err != nil {
		return err
	}

	// Turn this into an HTTP POST request
	req, err := http.NewRequest("POST", u.String(), &b)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	// And finally, submit the request to the log
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to publish to log endpoint (%s): %w", u, err)
	}
	if r.StatusCode != http.StatusOK {
		return errFromResponse("failed to submit to log", r)
	}
	return nil
}

// PublishAnnotationMalware publishes the serialized annotation to the log.
func (c SubmitClient) PublishAnnotationMalware(stmt []byte) error {
	u, err := c.LogURL.Parse(api.HTTPAddAnnotationMalware)
	if err != nil {
		return err
	}
	glog.V(1).Infof("Submitting to %v", u.String())
	r, err := http.Post(u.String(), "application/json", bytes.NewBuffer(stmt))
	if err != nil {
		return fmt.Errorf("failed to publish to log endpoint (%s): %w", u, err)
	}
	if r.StatusCode != http.StatusOK {
		return errFromResponse("failed to submit to log", r)
	}
	return nil
}

// GetCheckpoint returns a new LogCheckPoint from the server.
func (c ReadonlyClient) GetCheckpoint() (*api.LogCheckpoint, error) {
	u, err := c.LogURL.Parse(api.HTTPGetRoot)
	if err != nil {
		return nil, err
	}
	r, err := http.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return &api.LogCheckpoint{}, errFromResponse("failed to fetch checkpoint", r)
	}

	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	return api.ParseCheckpoint(b, c.LogSigVerifier)
}

// GetInclusion returns an inclusion proof for the statement under the given checkpoint.
func (c ReadonlyClient) GetInclusion(statement []byte, cp api.LogCheckpoint) (api.InclusionProof, error) {
	hash := rfc6962.DefaultHasher.HashLeaf(statement)
	u, err := c.LogURL.Parse(fmt.Sprintf("%s/for-leaf-hash/%s/in-tree-of/%d", api.HTTPGetInclusion, base64.URLEncoding.EncodeToString(hash), cp.Size))
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
// TODO(mhutchinson): Rename this as leaf values can also be annotations.
func (c ReadonlyClient) GetManifestEntryAndProof(request api.GetFirmwareManifestRequest) (*api.InclusionProof, error) {
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
func (c ReadonlyClient) GetConsistencyProof(request api.GetConsistencyRequest) (*api.ConsistencyProof, error) {
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

// GetFirmwareImage returns the firmware image with the corresponding hash from the personality CAS.
func (c ReadonlyClient) GetFirmwareImage(hash []byte) ([]byte, error) {
	url := fmt.Sprintf("%s/with-hash/%s", api.HTTPGetFirmwareImage, base64.URLEncoding.EncodeToString(hash))

	u, err := c.LogURL.Parse(url)
	if err != nil {
		return nil, err
	}

	r, err := http.Get(u.String())
	if err != nil {
		return nil, err
	}
	if r.StatusCode != 200 {
		return nil, errFromResponse("failed to fetch firmware image", r)
	}

	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read firmware image from response: %w", err)
	}

	return b, nil
}

func errFromResponse(m string, r *http.Response) error {
	if r.StatusCode == 200 {
		return nil
	}

	b, _ := io.ReadAll(r.Body) // Ignore any error, we want to ensure we return the right status code which we already know.

	msg := fmt.Sprintf("%s: %s", m, string(b))
	return status.New(codeFromHTTPResponse(r.StatusCode), msg).Err()
}
