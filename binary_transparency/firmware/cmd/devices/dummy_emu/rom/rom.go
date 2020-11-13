package rom

import (
	"bytes"
	"crypto/sha512"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/verify"
)

var (
	dummyDirectory = flag.String("dummy_storage_dir", "/tmp/dummy_device", "Directory path of the dummy device's state storage")
)

const (
	bundlePath   = "bundle.json"
	firmwarePath = "firmware.bin"
)

// Chain represents the next stage in the boot process.
type Chain func() error

// ResetFromFlags is intended to emulate the early stage boot process of a device.
//
// It's separate from the device emulator code to highlight that the process of
// verifying the local firmware/proofs/etc. could be done on-device at
// as part of the boot-ROM execution to provide some tamper-resistant properties
// to the firmware installed on the device.
//
// Other, real-world, devices with secure elements may be able to optimise this
// process by checking once and leveraging properties of the hardware.
//
// Returns the first link in the boot chain as a func.
func ResetFromFlags() (Chain, error) {
	glog.Info("----RESET----")
	glog.Info("Powering up bananas, configuring Romulans, feeding the watchdogs")

	glog.Infof("Configuring flash and loading FT artifacts from %q...", *dummyDirectory)

	fwFile := filepath.Clean(filepath.Join(*dummyDirectory, firmwarePath))
	bundleFile := filepath.Clean(filepath.Join(*dummyDirectory, bundlePath))

	b, err := readAndParseBundle(bundleFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read transparency bundle: %w", err)
	}

	// validate bundle
	if err := verify.Bundle(b); err != nil {
		return nil, fmt.Errorf("failed to verify bundle: %w", err)
	}

	var stmt api.FirmwareStatement
	if err := json.Unmarshal(b.ManifestStatement, &stmt); err != nil {
		return nil, fmt.Errorf("error parsing firmware metadata: %w", err)
	}
	// TODO(al): check signature on statement and checkpoints when they're added.

	var metadata api.FirmwareMetadata
	if err := json.Unmarshal(stmt.Metadata, &metadata); err != nil {
		return nil, fmt.Errorf("error parsing firmware metadata: %w", err)
	}

	// TODO(al): measure firmware and check against manifest expected measurement.
	// HACK: this should use the firmware measurement field, but it'll do for now.
	fw, err := ioutil.ReadFile(fwFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read firmware: %w", err)
	}
	fwHash := sha512.Sum512(fw)
	if !bytes.Equal(fwHash[:], metadata.FirmwareImageSHA512) {
		return nil, fmt.Errorf("firmware hash does not match manifest: %w", err)
	}

	// boot
	glog.Infof("Prepared to boot %s", stmt.Metadata)
	return func() error { return nil }, nil
}

func readAndParseBundle(bundleFile string) (api.ProofBundle, error) {
	bundleRaw, err := ioutil.ReadFile(bundleFile)
	if err != nil {
		return api.ProofBundle{}, fmt.Errorf("failed to read transparency bundle: %w", err)
	}
	var pb api.ProofBundle
	if err := json.Unmarshal(bundleRaw, &pb); err != nil {
		return api.ProofBundle{}, fmt.Errorf("failed to parse proof bundle file: %w", err)
	}
	return pb, nil
}
