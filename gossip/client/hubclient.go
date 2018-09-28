// Copyright 2018 Google Inc. All Rights Reserved.
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

// Package client is a library for clients of a Gossip Hub.
package client

import (
	"bytes"
	"context"
	"crypto"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/certificate-transparency-go/jsonclient"
	"github.com/google/certificate-transparency-go/tls"
	"github.com/google/certificate-transparency-go/x509"
	"github.com/google/trillian-examples/gossip/api"

	tcrypto "github.com/google/trillian/crypto"
)

const base10 = 10

// HubClient represents a client for a given Gossip Hub instance
type HubClient struct {
	jsonclient.JSONClient
	Hash crypto.Hash
}

// New constructs a new HubClient instance for a given uri.
func New(uri string, hc *http.Client, opts jsonclient.Options) (*HubClient, error) {
	hubClient, err := jsonclient.New(uri, hc, opts)
	if err != nil {
		return nil, err
	}
	return &HubClient{JSONClient: *hubClient, Hash: crypto.SHA256}, err
}

// AddSignedBlob adds a signed blob from a specified source ID, returning a signed
// gossip timestamp (SGT) from the hub.
func (c *HubClient) AddSignedBlob(ctx context.Context, sourceID string, data, sig []byte) (*api.SignedGossipTimestamp, error) {
	req := api.AddSignedBlobRequest{SourceID: sourceID, BlobData: data, SourceSignature: sig}
	var rsp api.AddSignedBlobResponse
	httpRsp, body, err := c.PostAndParseWithRetry(ctx, api.PathPrefix+api.AddSignedBlobPath, &req, &rsp)
	if err != nil {
		return nil, err
	}

	// Pre-build a RspError for all error cases below.
	rspErr := jsonclient.RspError{StatusCode: httpRsp.StatusCode, Body: body}

	// First check the signature if we can.
	if err := c.VerifySignature(rsp.TimestampedEntryData, rsp.HubSignature); err != nil {
		rspErr.Err = err
		return nil, rspErr
	}

	// Now it's safe to decode the TLS-encoded contents.
	sgt := api.SignedGossipTimestamp{HubSignature: rsp.HubSignature}
	if rest, err := tls.Unmarshal(rsp.TimestampedEntryData, &sgt.TimestampedEntry); err != nil {
		rspErr.Err = err
		return nil, rspErr
	} else if len(rest) > 0 {
		rspErr.Err = fmt.Errorf("trailing data (%d bytes) after TimestampedEntry", len(rest))
		return nil, rspErr
	}

	// Check the SGT matches what we uploaded.
	if got := string(sgt.TimestampedEntry.SourceID); got != sourceID {
		rspErr.Err = fmt.Errorf("SGT has source ID %s, want %s", got, sourceID)
		return nil, rspErr
	}
	if got := sgt.TimestampedEntry.BlobData; !bytes.Equal(got, data) {
		rspErr.Err = fmt.Errorf("SGT has head data %x, want %x", got, data)
		return nil, rspErr
	}
	// The returned signature need not match: an earlier submission of the same blob
	// may have used a different signature from the same key, as some signature mechanisms
	// (e.g. ECDSA) are not deterministic.
	return &sgt, nil
}

// GetSTH retrieves the current STH from the hub.
func (c *HubClient) GetSTH(ctx context.Context) (*api.SignedHubTreeHead, error) {
	var rsp api.GetSTHResponse
	httpRsp, body, err := c.GetAndParse(ctx, api.PathPrefix+api.GetSTHPath, nil, &rsp)
	if err != nil {
		return nil, err
	}

	// Pre-build a RspError for all error cases below.
	rspErr := jsonclient.RspError{StatusCode: httpRsp.StatusCode, Body: body}

	// First check the signature if we can.
	if err := c.VerifySignature(rsp.TreeHeadData, rsp.HubSignature); err != nil {
		rspErr.Err = err
		return nil, rspErr
	}

	// Now it's safe to decode the TLS-encoded contents.
	shth := api.SignedHubTreeHead{HubSignature: rsp.HubSignature}
	if rest, err := tls.Unmarshal(rsp.TreeHeadData, &shth.TreeHead); err != nil {
		rspErr.Err = err
		return nil, rspErr
	} else if len(rest) > 0 {
		rspErr.Err = fmt.Errorf("trailing data (%d bytes) after TreeHead", len(rest))
		return nil, rspErr
	}
	return &shth, nil
}

// VerifySignature checks the signature in sth, returning any error encountered or nil if verification is
// successful.
func (c *HubClient) VerifySignature(data, sig []byte) error {
	if c.Verifier == nil {
		// Can't verify signatures without a verifier
		return nil
	}
	return tcrypto.Verify(c.Verifier.PubKey, c.Hash, data, sig)
}

// GetSTHConsistency retrieves the consistency proof between two hub tree heads.
func (c *HubClient) GetSTHConsistency(ctx context.Context, first, second uint64) ([][]byte, error) {
	if second < first {
		return nil, fmt.Errorf("range inverted: second (%d) < first (%d)", second, first)
	}
	params := map[string]string{
		api.GetSTHConsistencyFirst:  strconv.FormatUint(first, base10),
		api.GetSTHConsistencySecond: strconv.FormatUint(second, base10),
	}
	var rsp api.GetSTHConsistencyResponse
	_, _, err := c.GetAndParse(ctx, api.PathPrefix+api.GetSTHConsistencyPath, params, &rsp)
	if err != nil {
		return nil, err
	}
	return rsp.Consistency, nil
}

// GetProofByHash returns an audit path for the hash of a timestamped entry in the hub
// at a particular tree size.
func (c *HubClient) GetProofByHash(ctx context.Context, hash []byte, treeSize uint64) (*api.GetProofByHashResponse, error) {
	params := map[string]string{
		api.GetProofByHashSize: strconv.FormatUint(treeSize, base10),
		api.GetProofByHashArg:  base64.StdEncoding.EncodeToString(hash),
	}
	var rsp api.GetProofByHashResponse
	_, _, err := c.GetAndParse(ctx, api.PathPrefix+api.GetProofByHashPath, params, &rsp)
	if err != nil {
		return nil, err
	}
	return &rsp, nil
}

func (c *HubClient) getEntriesRsp(ctx context.Context, start, end int64) (int, []byte, [][]byte, error) {
	if start < 0 {
		return -1, nil, nil, errors.New("start should be >= 0")
	}
	if end < start {
		return -1, nil, nil, errors.New("start should be <= end")
	}

	params := map[string]string{
		api.GetEntriesStart: strconv.FormatInt(start, base10),
		api.GetEntriesEnd:   strconv.FormatInt(end, base10),
	}

	var rsp api.GetEntriesResponse
	httpRsp, body, err := c.GetAndParse(ctx, api.PathPrefix+api.GetEntriesPath, params, &rsp)
	if err != nil {
		return -1, nil, nil, err
	}
	return httpRsp.StatusCode, body, rsp.Entries, nil
}

// GetRawEntries returns the TLS-encoded TimestampedEntries in the hub for the given range.  Note that the
// requested range may be truncated.
func (c *HubClient) GetRawEntries(ctx context.Context, start, end int64) ([][]byte, error) {
	_, _, entries, err := c.getEntriesRsp(ctx, start, end)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// GetEntries returns the TimestampedEntries in the hub for the given range.  Note that the
// requested range may be truncated.
func (c *HubClient) GetEntries(ctx context.Context, start, end int64) ([]*api.TimestampedEntry, error) {
	status, body, rawEntries, err := c.getEntriesRsp(ctx, start, end)
	if err != nil {
		return nil, err
	}

	entries := make([]*api.TimestampedEntry, len(rawEntries))
	for i, data := range rawEntries {
		var entry api.TimestampedEntry
		if rest, err := tls.Unmarshal(data, &entry); err != nil {
			return nil, jsonclient.RspError{Err: err, StatusCode: status, Body: body}
		} else if len(rest) > 0 {
			return nil, jsonclient.RspError{
				Err:        fmt.Errorf("trailing data (%d bytes) after TimestampedEntry[%d]", len(rest), i),
				StatusCode: status,
				Body:       body,
			}
		}
		entries[i] = &entry
	}

	return entries, nil
}

// GetSourceKeys retrieves the set of source log IDs and public keys for a hub.
func (c *HubClient) GetSourceKeys(ctx context.Context) ([]*api.SourceKey, error) {
	var rsp api.GetSourceKeysResponse
	_, _, err := c.GetAndParse(ctx, api.PathPrefix+api.GetSourceKeysPath, nil, &rsp)
	if err != nil {
		return nil, err
	}
	return rsp.Entries, nil
}

// AcceptableSource checks whether a given public key is included in the set of
// source keys for a hub.
func AcceptableSource(pubKey crypto.PublicKey, srcKeys []*api.SourceKey) bool {
	pubDER, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		return false
	}
	for _, srcKey := range srcKeys {
		if bytes.Equal(pubDER, srcKey.PubKey) {
			return true
		}
	}
	return false
}

// GetLatestForSource retrieves the 'latest' entry for a source.
func (c *HubClient) GetLatestForSource(ctx context.Context, sourceID string) (*api.TimestampedEntry, error) {
	var rsp api.GetLatestForSourceResponse
	params := map[string]string{api.GetLatestForSourceID: sourceID}
	httpRsp, body, err := c.GetAndParse(ctx, api.PathPrefix+api.GetLatestForSourcePath, params, &rsp)
	if err != nil {
		return nil, err
	}

	var entry api.TimestampedEntry
	if rest, err := tls.Unmarshal(rsp.Entry, &entry); err != nil {
		return nil, jsonclient.RspError{Err: err, StatusCode: httpRsp.StatusCode, Body: body}
	} else if len(rest) > 0 {
		return nil, jsonclient.RspError{
			Err:        fmt.Errorf("trailing data (%d bytes) after TimestampedEntry", len(rest)),
			StatusCode: httpRsp.StatusCode,
			Body:       body,
		}
	}

	// Check it comes from the expected source.
	if !bytes.Equal(entry.SourceID, []byte(sourceID)) {
		return nil, fmt.Errorf("entry from unexpected source %q returned for %q", string(entry.SourceID), sourceID)
	}

	return &entry, nil
}
