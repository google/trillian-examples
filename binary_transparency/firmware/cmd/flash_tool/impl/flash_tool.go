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

// Package impl is the implementation of a util to flash firmware update packages created by the publisher tool onto devices.
//
// Currently, the only device is a dummy device, which simply sorts the firmware+metadata on local disk.
package impl

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/cmd/flash_tool/devices"
	"github.com/google/trillian-examples/binary_transparency/firmware/devices/dummy"
	armory_flash "github.com/google/trillian-examples/binary_transparency/firmware/devices/usbarmory/flash"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/client"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/verify"
	"github.com/google/trillian/merkle/coniks"
	"github.com/google/trillian/merkle/smt/node"
	"golang.org/x/mod/sumdb/note"
)

// FlashOpts encapsulates flash tool parameters.
type FlashOpts struct {
	DeviceID       string
	LogURL         string
	LogSigVerifier note.Verifier
	MapURL         string
	WitnessURL     string
	UpdateFile     string
	Force          bool
	DeviceStorage  string
}

// Main flashes the device according to the options provided.
func Main(ctx context.Context, opts FlashOpts) error {
	logURL, err := url.Parse(opts.LogURL)
	if err != nil {
		return fmt.Errorf("log_url is invalid: %w", err)
	}
	c := &client.ReadonlyClient{LogURL: logURL}

	up, err := readUpdateFile(opts.UpdateFile)
	if err != nil {
		return fmt.Errorf("failed to read update package file: %w", err)
	}

	dev, err := getDevice(opts)
	if err != nil {
		return fmt.Errorf("failed to get device: %w", err)
	}

	pb, fwMeta, err := verifyUpdate(c, opts.LogSigVerifier, up, dev)
	if err != nil {
		err := fmt.Errorf("failed to validate update: %w", err)
		if !opts.Force {
			return err
		}
		glog.Warning(err)
	}

	if len(opts.WitnessURL) > 0 {
		err := verifyWitness(c, opts.LogSigVerifier, pb, opts.WitnessURL)
		if err != nil {
			if !opts.Force {
				return err
			}
			glog.Warning(err)
		}
	}

	if len(opts.MapURL) > 0 {
		err := verifyAnnotations(ctx, c, opts.LogSigVerifier, pb, fwMeta, opts.MapURL)
		if err != nil {
			if !opts.Force {
				return fmt.Errorf("verifyAnnotations: %w", err)
			}
			glog.Warning(err)
		}
	}
	glog.Info("Update verified, about to apply to device...")

	if err := dev.ApplyUpdate(up); err != nil {
		return fmt.Errorf("failed to apply update to device: %w", err)
	}

	glog.Info("Update applied.")
	return nil
}

func getDevice(opts FlashOpts) (devices.Device, error) {
	var dev devices.Device
	var err error
	switch opts.DeviceID {
	case "armory":
		dev, err = armory_flash.New(opts.DeviceStorage)
	case "dummy":
		dev, err = dummy.New(opts.DeviceStorage)
	default:
		return dev, errors.New("device must be one of: 'dummy', 'armory'")
	}
	if err != nil {
		switch t := err.(type) {
		case devices.ErrNeedsInit:
			err := fmt.Errorf("device needs to be force initialised: %w", err)
			if !opts.Force {
				return dev, err
			}
			glog.Warning(err)
		default:
			err := fmt.Errorf("failed to open device: %w", t)
			if !opts.Force {
				return dev, err
			}
			glog.Warning(err)
		}
	}
	return dev, nil
}

func readUpdateFile(path string) (api.UpdatePackage, error) {
	if len(path) == 0 {
		return api.UpdatePackage{}, errors.New("must specify update_file")
	}

	f, err := os.OpenFile(path, os.O_RDONLY, os.ModePerm)
	if err != nil {
		glog.Exitf("Failed to open update package file %q: %q", path, err)
	}
	defer f.Close()

	var up api.UpdatePackage
	if err := json.NewDecoder(f).Decode(&up); err != nil {
		glog.Exitf("Failed to parse update package file: %q", err)
	}
	return up, nil
}

// getConsistencyFunc executes on a given client context and returns a
// consistency function.
func getConsistencyFunc(c *client.ReadonlyClient) func(from, to uint64) ([][]byte, error) {
	cpFunc := func(from, to uint64) ([][]byte, error) {
		var cp [][]byte
		if from > 0 {
			r, err := c.GetConsistencyProof(api.GetConsistencyRequest{From: from, To: to})
			if err != nil {
				return nil, fmt.Errorf("failed to fetch consistency proof: %w", err)
			}
			cp = r.Proof
		}
		return cp, nil
	}
	return cpFunc
}

// verifyUpdate checks that an update package is self-consistent and returns a verified proof bundle
func verifyUpdate(c *client.ReadonlyClient, logSigVerifier note.Verifier, up api.UpdatePackage, dev devices.Device) (api.ProofBundle, api.FirmwareMetadata, error) {
	var pb api.ProofBundle
	var fwMeta api.FirmwareMetadata

	// Get the consistency proof for the bundle
	n, err := dev.DeviceCheckpoint()
	if err != nil {
		return pb, fwMeta, fmt.Errorf("failed to fetch the device checkpoint: %w", err)
	}
	dc, err := api.ParseCheckpoint([]byte(n), logSigVerifier)
	if err != nil {
		return pb, fwMeta, fmt.Errorf("failed to open the device checkpoint: %w", err)
	}

	cpFunc := getConsistencyFunc(c)
	fwHash := sha512.Sum512(up.FirmwareImage)
	pb, fwMeta, err = verify.BundleForUpdate(up.ProofBundle, fwHash[:], *dc, cpFunc, logSigVerifier)
	if err != nil {
		return pb, fwMeta, fmt.Errorf("failed to verify proof bundle: %w", err)
	}
	return pb, fwMeta, nil
}

func verifyWitness(c *client.ReadonlyClient, logSigVerifier note.Verifier, pb api.ProofBundle, witnessURL string) error {
	wURL, err := url.Parse(witnessURL)
	if err != nil {
		return fmt.Errorf("witness_url is invalid: %w", err)
	}
	wc := client.WitnessClient{
		URL:            wURL,
		LogSigVerifier: logSigVerifier,
	}

	wcp, err := wc.GetWitnessCheckpoint()
	if err != nil {
		return fmt.Errorf("failed to fetch the witness checkpoint: %w", err)
	}
	if wcp.Size == 0 {
		return fmt.Errorf("no witness checkpoint to verify")
	}
	if err := verify.BundleConsistency(pb, *wcp, getConsistencyFunc(c), logSigVerifier); err != nil {
		return fmt.Errorf("failed to verify checkpoint consistency against witness: %w", err)
	}
	return nil
}

func verifyAnnotations(ctx context.Context, c *client.ReadonlyClient, logSigVerifier note.Verifier, pb api.ProofBundle, fwMeta api.FirmwareMetadata, mapURL string) error {
	mc, err := client.NewMapClient(mapURL)
	if err != nil {
		return fmt.Errorf("failed to create map client: %w", err)
	}
	mcp, err := mc.MapCheckpoint()
	if err != nil {
		return fmt.Errorf("failed to get map checkpoint: %w", err)
	}
	// The map checkpoint should be stored in a log, and this client should follow
	// the log, checking consistency proofs and maintaining a golden checkpoint.
	// Without this, the client is at risk of being given a custom map root that
	// nobody else in the world sees.
	glog.V(1).Infof("Received map checkpoint: %s", mcp.LogCheckpoint)
	var lcp api.LogCheckpoint
	if err := json.Unmarshal(mcp.LogCheckpoint, &lcp); err != nil {
		return fmt.Errorf("failed to unmarshal log checkpoint: %w", err)
	}
	// TODO(mhutchinson): check consistency with the largest checkpoint found thus far
	// in order to detect a class of fork; it could be that the checkpoint in the update
	// is consistent with the map and the witness, but the map and the witness aren't
	// consistent with each other.
	if err := verify.BundleConsistency(pb, lcp, getConsistencyFunc(c), logSigVerifier); err != nil {
		return fmt.Errorf("failed to verify update with map checkpoint: %w", err)
	}

	// Get the aggregation and proof, and then check everything about it.
	// This is a little paranoid as the inclusion proof is generated client-side.
	// The pretense here is that there is a trust boundary between the code in this class,
	// and everything else. In a production system, it is likely that the proof generation
	// would live elsewhere (e.g. in the OTA packaging process), and the shorter proof
	// bundle would be provided to the device.
	preimage, ip, err := mc.Aggregation(ctx, mcp.Revision, pb.InclusionProof.LeafIndex)
	if err != nil {
		return fmt.Errorf("failed to get map value for %q: %w", pb.InclusionProof.LeafIndex, err)
	}
	// 1. Check: the proof is for the correct key.
	kbs := sha512.Sum512_256([]byte(fmt.Sprintf("summary:%d", pb.InclusionProof.LeafIndex)))
	if !bytes.Equal(ip.Key, kbs[:]) {
		return fmt.Errorf("received inclusion proof for key %x but wanted %x", ip.Key, kbs[:])
	}
	// 2. Check: the proof is for the correct value.
	leafID := node.NewID(string(kbs[:]), 256)
	hasher := coniks.Default
	expectedCommitment := hasher.HashLeaf(api.MapTreeID, leafID, preimage)
	if !bytes.Equal(ip.Value, expectedCommitment) {
		// This could happen if the JSON roundtripping was not stable. If we see that happen,
		// then we'll need to pass out the raw bytes received from the server and parse into
		// the struct at a higher level.
		// It could also happen because the value returned is not actually committed to by the map.
		return fmt.Errorf("received inclusion proof for value %x but wanted %x", ip.Value, expectedCommitment)
	}
	// 3. Check: the inclusion proof evaluates to the map root that we've obtained.
	// The calculation starts from the leaf, and uses the siblings from the inclusion proof
	// to generate the root, which is then compared with the map checkpoint.
	calc := expectedCommitment
	for pd := hasher.BitLen(); pd > 0; pd-- {
		sib := ip.Proof[pd-1]
		stem := leafID.Prefix(uint(pd))
		if sib == nil {
			sib = hasher.HashEmpty(api.MapTreeID, stem.Sibling())
		}
		left, right := calc, sib
		if !isLeftChild(stem) {
			left, right = right, left
		}
		calc = hasher.HashChildren(left, right)
	}
	if !bytes.Equal(calc, mcp.RootHash) {
		return fmt.Errorf("inclusion proof calculated root %x but wanted %x", calc, mcp.RootHash)
	}

	var agg api.AggregatedFirmware
	if err := json.Unmarshal(preimage, &agg); err != nil {
		return fmt.Errorf("failed to decode aggregation: %w", err)
	}
	// Now we're certain that the aggregation is contained in the map, we can use the value.
	if !agg.Good {
		return errors.New("firmware is marked as bad")
	}
	return nil
}

// isLeftChild returns whether the given node is a left child.
func isLeftChild(id node.ID) bool {
	last, bits := id.LastByte()
	return last&(1<<(8-bits)) == 0
}
