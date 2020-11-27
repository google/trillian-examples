//+build armory

package main

import (
	"encoding/json"
	"fmt"
	"io"
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

// hashPartition calculates the SHA256 of the first numBytes of the partition.
func hashPartition(numBytes int64, p *Partition) ([]byte, error) {
	fmt.Println("Reading partition at offset %d...", p.Offset)
	if _, err := p.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek: %w", err)
	}

	bs := int64(1<<20)
	rc := make(chan []byte, 10)
	hc := make(chan []byte)

	start := time.Now()
	go func() {
		h, err := dcp.New256()
		if err != nil {
			panic(fmt.Sprintf("Failed to created hasher: %q", err))
		}
		for b := range(rc) {
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
		rc<-b[:bc]

		numBytes -= int64(bc)
	}
	close(rc)

	fmt.Printf("Finished reading, hashing in %s\n", time.Now().Sub(start))
	hash := <-hc
	return hash[:], nil
}

