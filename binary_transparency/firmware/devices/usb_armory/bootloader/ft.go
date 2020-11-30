//+build armory

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/f-secure-foundry/tamago/soc/imx6/dcp"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	_ "github.com/google/trillian-examples/binary_transparency/firmware/internal/verify"
)

const bundlePath = "/bundle.json"

func init() {
	dcp.Init()
}

// loadBundle loads the proof bundle from the proof partition.
func loadBundle(p *Partition) (api.ProofBundle, error) {
	f, err := p.ReadAll(bundlePath)
	if err != nil {
		return api.ProofBundle{}, fmt.Errorf("failed to read bundle: %w", err)
	}

	var bundle api.ProofBundle
	if err := json.Unmarshal(f, &bundle); err != nil {
		return api.ProofBundle{}, fmt.Errorf("invalid bundle: %w", err)
	}
	return bundle, nil
}

// hashPartition calculates the SHA256 of the whole partition.
func hashPartition(p *Partition) ([]byte, error) {
	log.Printf("Reading partition at offset %d...\n", p.Offset)
	numBytes, err := p.GetExtFilesystemSize()
	if err != nil {
		return nil, fmt.Errorf("failed to get partition size: %w", err)
	}
	log.Printf("Partition size %d bytes\n", numBytes)

	if _, err := p.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek: %w", err)
	}

	bs := uint64(1 << 16)
	rc := make(chan []byte, 10)
	hc := make(chan []byte)

	start := time.Now()
	go func() {
		h, err := dcp.New256()
		if err != nil {
			panic(fmt.Sprintf("Failed to created hasher: %q", err))
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

	for numBytes > 0 {
		n := numBytes
		if n > bs {
			n = bs
		}
		b := make([]byte, bs)
		bc, err := p.Read(b)
		if err != nil {
			return nil, fmt.Errorf("failed to read: %w", err)
		}
		rc <- b[:bc]

		numBytes -= uint64(bc)
	}
	close(rc)

	log.Printf("Finished reading, hashing in %s\n", time.Now().Sub(start))
	hash := <-hc
	return hash[:], nil
}
