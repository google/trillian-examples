//+build armory

package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	_ "github.com/google/trillian-examples/binary_transparency/firmware/internal/verify"
)

const bundlePath = "/bundle.json"

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

// hashPartition calclates the SHA256 of the first numBytes of the partition.
func hashPartition(numBytes int, p *Partition) ([]byte, error) {
	fmt.Println("Reading partition...")
	if _, err := p.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek: %w", err)
	}

	bs := 1<<16
	rc := make(chan []byte, 10)
	hc := make(chan []byte)

	go func() {
		h := sha256.New()
		for b := range(rc) {
		if _, err := h.Write(b); err != nil {
			panic(fmt.Errorf("failed to hash: %w", err))
		}
		}
		hash := h.Sum(nil)
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

		numBytes -= bc
	}
	close(rc)

	fmt.Println("finished reading, hashing")
	hash := <-hc
	return hash[:], nil
}

