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
	"flag"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/witness/golang/omniwitness/internal/omniwitness"
	"github.com/google/trillian-examples/witness/golang/omniwitness/usbarmory/internal/storage"
	"github.com/google/trillian-examples/witness/golang/omniwitness/usbarmory/internal/storage/slots"
	"golang.org/x/mod/sumdb/note"
	"golang.org/x/sync/errgroup"

	usbarmory "github.com/usbarmory/tamago/board/usbarmory/mk2"
	"github.com/usbarmory/tamago/soc/nxp/imx6ul"
)

const (
	// Timeout for any http requests.
	httpTimeout = 10 * time.Second

	// Generated from https://go.dev/play/p/uWUKLNK6h9v
	// TODO(mhutchinson): these need to be read from file instead of constants
	publicKey  = "TrustMe+68958214+AQ4Ys/PsXqfhPkNK7Y7RyYUMOJvfl65PzJOEiq9VFPjF"
	signingKey = "PRIVATE+KEY+TrustMe+68958214+AZKby3TDZizdARF975ZyLJwGbHTivd+EqbfYTN5qr2cI"

	// slotsPartitionOffsetBytes defines where our witness data storage partition starts.
	// Changing this location is overwhelmingly likely to result in data loss.
	slotsPartitionOffsetBytes = 1 << 30
	// slotsPartitionLengthBytes specifies the size of the slots partition.
	// Increasing this value is relatively safe, if you're sure there is no data
	// stored in blocks which follow the current partition.
	//
	// We're starting with enough space for 1024 slots of 1MB each.
	slotsPartitionLengthBytes = 1024 * slotSizeBytes

	// slotSizeBytes is the size of each individual slot in the partition.
	slotSizeBytes = 1 << 20
)

func init() {
	debugConsole, _ := usbarmory.DetectDebugAccessory(250 * time.Millisecond)
	<-debugConsole

	imx6ul.DCP.Init()

	if err := usbarmory.MMC.Detect(); err != nil {
		glog.Exitf("Failed to detect MMC: %v", err)
	}

}

// dcpSHA256Mu serialises access to the dcpSHA256 function.
var dcpSHA256Mu = sync.Mutex{}

// dcpSHA256 uses the DCP on the USBArmory for calculating SHA256 hashes of the
// bytes available via the passed in Reader.
// Using the DCP is faster and uses less power than computing SHA256 on the CPU,
// however only one hash can be computed at a time.
func dcpSHA256(r io.Reader) ([32]byte, error) {
	dcpSHA256Mu.Lock()
	defer dcpSHA256Mu.Unlock()

	var ret [32]byte
	h, err := imx6ul.DCP.New256()
	if err != nil {
		return ret, fmt.Errorf("failed to create DCP hasher: %v", err)
	}
	bs := 32 << 10
	buf := make([]byte, bs)
	var n int
	for err == nil {
		n, err = r.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
		}
	}
	s, err := h.Sum(nil)
	if err != nil {
		return ret, err
	}
	copy(ret[:], s)
	return ret, nil
}

func main() {
	// We parse the flags despite declaring none ourselves so libraries are
	// happy (looking at you, glog).
	flag.Set("stderrthreshold", "INFO")
	flag.Set("v", "1")
	flag.Set("vmodule", "journal=1")
	flag.Parse()

	glog.Infof("Opening storage...")
	part := openStorage()
	glog.Infof("Storage opened.")

	// Set this to true to "wipe" the storage.
	// Currently this simply force-writes an entry with zero bytes to
	// each known slot.
	// If the journal(s) become corrupt a larger hammer will be required.
	reinit := false
	if reinit {
		if err := part.Erase(); err != nil {
			glog.Exitf("Failed to erase partition: %v", err)
		}
	}

	ctx := context.Background()
	// This error group will be used to run all top level processes
	g := errgroup.Group{}

	if err := initNetworking(); err != nil {
		glog.Exitf("Failed to init usb networking: %v", err)
	}

	// TODO(mhutchinson): add a second listener for an admin API.
	mainListener, err := iface.ListenerTCP4(80)
	if err != nil {
		glog.Exitf("could not initialize HTTP listener: %v", err)
	}

	g.Go(runNetworking)
	httpClient := getHttpClient()

	signer, err := note.NewSigner(signingKey)
	if err != nil {
		glog.Exitf("Failed to init signer: %v", err)
	}
	verifier, err := note.NewVerifier(publicKey)
	if err != nil {
		glog.Exitf("Failed to init verifier: %v", err)
	}
	opConfig := omniwitness.OperatorConfig{
		WitnessSigner:   signer,
		WitnessVerifier: verifier,
	}
	p := storage.NewSlotPersistence(part)
	if err := p.Init(); err != nil {
		glog.Exitf("Failed to create persistence layer: %v", err)
	}
	if err := omniwitness.Main(ctx, opConfig, p, mainListener, httpClient); err != nil {
		glog.Exitf("Main failed: %v", err)
	}
}

func openStorage() *slots.Partition {
	// dev is our access to the MMC storage.
	dev := &storage.Device{
		Card: usbarmory.MMC,
	}
	bs := uint(usbarmory.MMC.Info().BlockSize)
	geo := slots.Geometry{
		Start:  slotsPartitionOffsetBytes / bs,
		Length: slotsPartitionLengthBytes / bs,
	}
	sl := slotSizeBytes / bs
	for i := uint(0); i < geo.Length; i += sl {
		geo.SlotLengths = append(geo.SlotLengths, sl)
	}

	p, err := slots.OpenPartition(dev, geo, dcpSHA256)
	if err != nil {
		glog.Exitf("Failed to open partition: %v", err)
	}
	return p
}
