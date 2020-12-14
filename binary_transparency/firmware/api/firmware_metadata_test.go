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

// Package api_test holds blackbox tests for the api package.
package api_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
)

func TestFirmwareMetadataToString(t *testing.T) {
	for _, test := range []struct {
		desc string
		fm   api.FirmwareMetadata
		r    *regexp.Regexp
	}{
		{
			desc: "one",
			fm: api.FirmwareMetadata{
				DeviceID:                    "device",
				FirmwareRevision:            1,
				FirmwareImageSHA512:         []byte("this is an image hash"),
				ExpectedFirmwareMeasurement: []byte("this is a firmware measurement"),
				BuildTimestamp:              "noon",
			},
			r: regexp.MustCompile(fmt.Sprintf("device.*v1.*noon.*%x", "this is an image hash")),
		},
		{
			desc: "two",
			fm: api.FirmwareMetadata{
				DeviceID:                    "banana",
				FirmwareRevision:            100,
				FirmwareImageSHA512:         []byte("an image hash this is"),
				ExpectedFirmwareMeasurement: []byte("a measurement this is"),
				BuildTimestamp:              "ouch",
			},
			r: regexp.MustCompile(fmt.Sprintf("banana.*v100.*ouch.*%x", "an image hash this is")),
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			if got := test.fm.String(); !test.r.Match([]byte(got)) {
				t.Fatalf("Failed to match output %q", got)
			}
		})
	}
}
