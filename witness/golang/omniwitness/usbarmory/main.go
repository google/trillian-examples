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
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/witness/golang/omniwitness/internal/omniwitness"
	"golang.org/x/mod/sumdb/note"
	"golang.org/x/sync/errgroup"

	_ "github.com/usbarmory/tamago/board/f-secure/usbarmory/mark-two"
)

const (
	// Timeout for any http requests.
	httpTimeout = 10 * time.Second

	// Generated from https://go.dev/play/p/uWUKLNK6h9v
	// TODO(mhutchinson): these need to be read from file instead of constants
	publicKey  = "TrustMe+68958214+AQ4Ys/PsXqfhPkNK7Y7RyYUMOJvfl65PzJOEiq9VFPjF"
	signingKey = "PRIVATE+KEY+TrustMe+68958214+AZKby3TDZizdARF975ZyLJwGbHTivd+EqbfYTN5qr2cI"
)

func main() {
	// We parse the flags despite declaring none ourselves so libraries are
	// happy (looking at you, glog).
	flag.Parse()
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
	verifier, err := note.NewVerifier(publicKey)
	if err != nil {
		glog.Exitf("Failed to init verifier: %v", err)
	}
	opConfig := omniwitness.OperatorConfig{
		WitnessSigner:   signer,
		WitnessVerifier: verifier,
	}
	if err := omniwitness.Main(ctx, opConfig, httpListener, httpClient); err != nil {
		glog.Exitf("Main failed: %v", err)
	}
}
