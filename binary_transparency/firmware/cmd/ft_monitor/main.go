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

// This package is the entrypoint for the Firmware Transparency monitor.
// The monitor follows the growth of the Firmware Transparency log server,
// inspects new firmware metadata as it appears, and prints out a short
// summary.
//
// TODO(al): Extend monitor to verify claims.
//
// Start the monitor using:
// go run ./cmd/ft_monitor/main.go --logtostderr -v=2 --ftlog=http://localhost:8000/
package main

import (
	"flag"
	"net/url"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/client"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/verify"
)

var (
	ftLog        = flag.String("ftlog", "http://localhost:8000", "Base URL of FT Log server")
	pollInterval = flag.Duration("poll_interval", 5*time.Second, "Duration to wait between polling for new entries")
)

func main() {
	flag.Parse()

	if len(*ftLog) == 0 {
		glog.Exit("ftlog is required")
	}

	ftURL, err := url.Parse(*ftLog)
	if err != nil {
		glog.Exitf("Failed to parse FT log URL: %q", err)
	}

	glog.Infof("Monitoring FT log %q...", *ftLog)
	ticker := time.NewTicker(*pollInterval)

	c := client.Client{LogURL: ftURL}
	var latestCP api.LogCheckpoint

	lv := verify.NewLogVerifier()

	for {
		<-ticker.C
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
}
