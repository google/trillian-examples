//+build armory

package main

import (
	"fmt"
	"io"
	"log"
	"time"

	"github.com/f-secure-foundry/tamago/soc/imx6/dcp"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/crypto"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/verify"
	"golang.org/x/mod/sumdb/note"
)

const (
	bundlePath                      = "/bundle.json"
	firmwareMeasurementDomainPrefix = "armory_mkii"
)

func init() {
	dcp.Init()
}

// verifyIntegrity checks the validity of the device state
// against the stored proof bundle.
//
// This method will fail if:
//   - there is no proof bundle stored in the proof partition
//   - the proof bundle is not self-consistent
//   - the measurement hash of the installed firmware does not
//     match the value expected by the firmware manifest
//   - TODO(al): check signatures.
func verifyIntegrity(proof, firmware *Partition) error {
	rawBundle, err := proof.ReadAll(bundlePath)
	if err != nil {
		return fmt.Errorf("failed to read bundle: %w", err)
	}

	h, err := measureFirmware(firmware)
	if err != nil {
		return fmt.Errorf("failed to hash firmware partition: %w\n", err)
	}
	fmt.Printf("firmware partition hash: 0x%x\n", h)
	cpSigVerifier, err := note.NewVerifier(crypto.TestFTPersonalityPub)

	if err := verify.BundleForBoot(rawBundle, h, note.VerifierList(cpSigVerifier)); err != nil {
		return fmt.Errorf("failed to verify bundle: %w", err)
	}
	return nil
}

// measureFirmware returns the firmware measurement hash for the firmware
// stored on the given partition.
func measureFirmware(p *Partition) ([]byte, error) {
	log.Printf("Reading partition at offset %d...\n", p.Offset)
	numBytes, err := p.GetExt4FilesystemSize()
	if err != nil {
		return nil, fmt.Errorf("failed to get partition size: %w", err)
	}
	log.Printf("Partition size %d bytes\n", numBytes)

	if _, err := p.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek: %w", err)
	}

	bs := uint64(1 << 21)
	rc := make(chan []byte, 5)
	hc := make(chan []byte)

	start := time.Now()
	go func() {
		h, err := dcp.New256()
		if err != nil {
			panic(fmt.Sprintf("Failed to created hasher: %q", err))
		}

		if _, err := h.Write([]byte(firmwareMeasurementDomainPrefix)); err != nil {
			panic(fmt.Sprintf("Failed to write measurement domain prefix: %q", err))
		}

		for b := range rc {
			if _, err := h.Write(b); err != nil {
				panic(fmt.Errorf("failed to hash: %w", err))
			}
		}
		hash, err := h.Sum(nil)
		if err != nil {
			panic(fmt.Sprintf("Failed to get final sum: %q", err))
		}
		hc <- hash
	}()

	if _, err := p.Seek(0, io.SeekStart); err != nil {
		panic(fmt.Sprintf("Failed to seek to start of partition: %q", err))
	}
	for numBytes > 0 {
		n := numBytes
		if n > bs {
			n = bs
		}
		b := make([]byte, n)
		bc, err := p.Read(b)
		if err != nil {
			return nil, fmt.Errorf("failed to read: %w", err)
		}
		rc <- b[:bc]

		numBytes -= uint64(bc)
	}
	close(rc)

	hash := <-hc

	log.Printf("Finished reading, hashing in %s\n", time.Now().Sub(start))
	return hash[:], nil
}
