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

// Package impl is the implementation of the Firmware Transparency monitor.
// The monitor follows the growth of the Firmware Transparency log server,
// inspects new firmware metadata as it appears, and prints out a short
// summary.
//
// TODO(al): Extend monitor to verify claims.
package impl

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/client"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/crypto"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/verify"
)

// MatchFunc is the signature of a function which can be called by the monitor
// to signal when it's found a keyword match in a logged entry.
type MatchFunc func(index uint64, fw api.FirmwareMetadata)

// MonitorOpts encapsulates options for running the monitor.
type MonitorOpts struct {
	LogURL       string
	PollInterval time.Duration
	Keyword      string
	Matched      MatchFunc
}

func Main(ctx context.Context, opts MonitorOpts) error {

	if len(opts.LogURL) == 0 {
		return errors.New("log URL is required")
	}

	ftURL, err := url.Parse(opts.LogURL)
	if err != nil {
		return fmt.Errorf("failed to parse FT log URL: %w", err)
	}

	glog.Infof("Monitoring FT log %q...", opts.LogURL)
	ticker := time.NewTicker(opts.PollInterval)

	c := client.ReadonlyClient{LogURL: ftURL}
	var latestCP api.LogCheckpoint

	lv := verify.NewLogVerifier()

	// Parse the input keywords as regular expression
	var ftKeywords []*regexp.Regexp
	for _, k := range strings.Fields(opts.Keyword) {
		ftKeywords = append(ftKeywords, regexp.MustCompile(k))
	}

	for {
		select {
		case <-ticker.C:
			//
		case <-ctx.Done():
			return ctx.Err()
		}

		cp, err := c.GetCheckpoint()
		if err != nil {
			glog.Warningf("Failed to update LogCheckpoint: %q", err)
			continue
		}
		// TODO(al): check signature on checkpoint when they're added.

		if cp.TreeSize <= latestCP.TreeSize {
			continue
		}
		glog.V(1).Infof("Got newer checkpoint %s", cp)

		// Fetch all the manifests from now till new checkpoint
		for idx := latestCP.TreeSize; idx < cp.TreeSize; idx++ {

			manifest, err := c.GetManifestEntryAndProof(api.GetFirmwareManifestRequest{Index: idx, TreeSize: cp.TreeSize})
			if err != nil {
				glog.Warningf("Failed to fetch the Manifest: %q", err)
				continue
			}
			glog.V(1).Infof("Received New Manifest with Information=")
			glog.V(1).Infof("Manifest Value = %s", manifest.Value)
			glog.V(1).Infof("LeafIndex = %d", manifest.LeafIndex)
			glog.V(1).Infof("Proof = %x", manifest.Proof)

			lh := verify.HashLeaf(manifest.Value)
			if err := lv.VerifyInclusionProof(int64(manifest.LeafIndex), int64(cp.TreeSize), manifest.Proof, cp.RootHash, lh); err != nil {
				// Report Failed Inclusion Proof
				glog.Warningf("Invalid inclusion proof received for LeafIndex %d", manifest.LeafIndex)
				continue
			}

			glog.V(1).Infof("Inclusion proof for leafhash 0x%x verified", lh)

			statement := manifest.Value
			stmt := api.SignedStatement{}
			if err := json.NewDecoder(bytes.NewReader(statement)).Decode(&stmt); err != nil {
				glog.Warningf("SignedStatement decoding failed: %q", err)
				continue
			}

			// Verify the signature:
			if err := crypto.Publisher.VerifySignature(stmt.Type, stmt.Statement, stmt.Signature); err != nil {
				glog.Warningf("Signature verification failed: %q", err)
				continue
			}
			glog.V(1).Infof("Signature verification SUCCESS")

			if stmt.Type != api.FirmwareMetadataType {
				// TODO(mhutchinson): Process annotations in the monitor?
				continue
			}

			// Parse the firmware metadata:
			var meta api.FirmwareMetadata
			if err := json.Unmarshal(stmt.Statement, &meta); err != nil {
				glog.Warningf("Unable to decode FW Metadata from Statement %q", err)
				continue
			}

			glog.Infof("Found firmware (@%d): %s", idx, meta)

			// Fetch the Image from FT Personality
			image, err := c.GetFirmwareImage(meta.FirmwareImageSHA512)
			if err != nil {
				glog.Warningf("Unable to GetFirmwareImage for Firmware with Hash 0x%x , reason %q", meta.FirmwareImageSHA512, err)
				continue
			}
			// Verify Image Hash from log Manifest matches the actual image hash
			h := sha512.Sum512(image)
			if !bytes.Equal(h[:], meta.FirmwareImageSHA512) {
				glog.Warningf("downloaded image does not match SHA512 in metadata (%x != %x)", h[:], meta.FirmwareImageSHA512)
				continue
			}
			glog.V(1).Infof("Image Hash Verified for image at leaf index %d", manifest.LeafIndex)

			//Search for specific keywords inside firmware image
			for _, m := range ftKeywords {
				if m.Match(image) {
					opts.Matched(idx, meta)
				}
			}
		}

		// Perform consistency check only for non-zero initial tree size
		if latestCP.TreeSize != 0 {
			consistency, err := c.GetConsistencyProof(api.GetConsistencyRequest{From: latestCP.TreeSize, To: cp.TreeSize})
			if err != nil {
				glog.Warningf("Failed to fetch the Consistency: %q", err)
				continue
			}
			glog.V(1).Infof("Printing the latest Consistency Proof Information")
			glog.V(1).Infof("Consistency Proof = %x", consistency.Proof)

			//Verify the fetched consistency proof
			if err := lv.VerifyConsistencyProof(int64(latestCP.TreeSize), int64(cp.TreeSize), latestCP.RootHash, cp.RootHash, consistency.Proof); err != nil {
				// Verification of Consistency Proof failed!!
				glog.Warningf("Failed verification of Consistency proof %q", err)
				continue
			}
			glog.V(1).Infof("Consistency proof for Treesize %d verified", cp.TreeSize)
		}

		latestCP = *cp
	}
	// unreachable
}
