// Copyright 2022 Google LLC. All Rights Reserved.
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

//go:build usbarmory
// +build usbarmory

// usbarmory is the omniwitness composed as a unikernel to be deployed on
// the USB Armory MK II.
// To build this, `make CROSS_COMPILE=arm-none-eabi- imx` and then flash
// the imx file to the device.
package main

import (
	"context"
	"crypto/rand"
	"flag"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/witness/golang/omniwitness/internal/omniwitness"
	"golang.org/x/mod/sumdb/note"
	"golang.org/x/sync/errgroup"

	"github.com/google/trillian-examples/witness/golang/omniwitness/usbarmory/storage"
	"github.com/google/trillian-examples/witness/golang/omniwitness/usbarmory/storage/slots"

	usbarmory "github.com/usbarmory/tamago/board/f-secure/usbarmory/mark-two"
)

const (
	// Timeout for any http requests.
	httpTimeout = 10 * time.Second

	// Generated from https://go.dev/play/p/uWUKLNK6h9v
	// TODO(mhutchinson): these need to be read from file instead of constants
	publicKey  = "TrustMe+68958214+AQ4Ys/PsXqfhPkNK7Y7RyYUMOJvfl65PzJOEiq9VFPjF"
	signingKey = "PRIVATE+KEY+TrustMe+68958214+AZKby3TDZizdARF975ZyLJwGbHTivd+EqbfYTN5qr2cI"
	_          = publicKey // public key is here so it doesn't get lost, we don't need it right now.
)

func main() {
	// We parse the flags despite declaring none ourselves so libraries are
	// happy (looking at you, glog).
	flag.Parse()

	if err := usbarmory.MMC.Detect(); err != nil {
		glog.Exitf("Failed to detect MMC: %v", err)
	}

	// dev is our access to the MMC storage.
	dev := &storage.Device{
		Card: usbarmory.MMC,
	}

	// geo defines the location and layout of our storage partition.
	geo := slots.Geometry{
		// Start at block 10, for no particular reason
		Start: 10,
		// Length of ths partition is 100 blocks, because it just is.
		Length: 100,
		// SlotLengths defines the layout of slots within our partition.
		// Slots are stored contiguously in the range of blocks [Start, Start+Length).
		// We've configured 3 slots, each with 10 blocks to hold its journal.
		// The remaining 70 blocks are unused.
		SlotLengths: []uint{
			10,
			10,
			10,
		},
	}
	p, err := slots.OpenPartition(dev, geo)
	if err != nil {
		glog.Exitf("Failed to open partition: %v", err)
	}

	s, err := p.Open(0)
	if err != nil {
		glog.Exitf("Failed to open slot 0: %v", err)
	}

	data, token, err := s.Read()
	if err != nil {
		glog.Exitf("Failed to read slot 0 data: %v", err)
	}

	newData := make([]byte, 10)
	if _, err := rand.Read(newData); err != nil {
		glog.Exitf("Failed to get random data: %v", err)
	}

	data = append(data, newData...)
	if err := s.CheckAndWrite(token, data); err != nil {
		glog.Exitf("Failed to update data: %v", err)
	}
	glog.Infof("Data@%x:\n%x", token, data)

	ctx := context.Background()
	// This error group will be used to run all top level processes
	g := errgroup.Group{}

	httpListener, err := initNetworking()
	if err != nil {
		glog.Exitf("Failed to init usb networking: %v", err)
	}
	g.Go(runNetworking)
	httpClient := getHttpClient()

	signer, err := note.NewSigner(signingKey)
	if err != nil {
		glog.Exitf("Failed to init signer: %v", err)
	}
	if err := omniwitness.Main(ctx, signer, httpListener, httpClient); err != nil {
		glog.Exitf("Main failed: %v", err)
	}
}
