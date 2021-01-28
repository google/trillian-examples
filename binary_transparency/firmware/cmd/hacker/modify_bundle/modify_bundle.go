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

// modify_bundle is a hacker tool for modifying proof bundles.
//
// It can update the expected measurement and firmware image hashes of the specified
// proof bundle file, optionally sign the firmware statement, and finally write the bundle
// to the specified output file.
//
// Usage:
//   go run ./cmd/hacker/modify_bundle/ \
//      --logtostderr \
//      --device=[dummy,usbarmory] \
//      --binary=/path/to/new/firmware/image \
//      --input=/path/to/bundle.json \
//      --output=/path/to/bundle.json
package main

import (
	"flag"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/cmd/hacker/modify_bundle/impl"
)

var (
	binaryPath = flag.String("binary", "", "Replacement binary image.")
	deviceID   = flag.String("device", "", "One of [dummy,usbarmory].")
	input      = flag.String("input", "", "File path read input ProofBundle from.")
	output     = flag.String("output", "", "File path to write output ProofBundle to.")
	sign       = flag.Bool("sign", true, "Whether to use stolen key to sign manifest.")
)

func main() {
	flag.Parse()

	if err := impl.Main(impl.ModifyBundleOpts{
		BinaryPath: *binaryPath,
		DeviceID:   *deviceID,
		Input:      *input,
		Output:     *output,
		Sign:       *sign,
	}); err != nil {
		glog.Exit(err.Error())
	}
}
