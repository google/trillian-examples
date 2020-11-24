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
// limitations under the License.package bundle_test

package verify_test

import (
	"crypto/sha512"
	"encoding/json"
	"testing"

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/verify"
)

const (
	goldenUpdate        = `{"FirmwareImage":"RmlybXdhcmUgaW1hZ2U=","ProofBundle":{"ManifestStatement":"eyJNZXRhZGF0YSI6ImV5SkVaWFpwWTJWSlJDSTZJbFJoYkd0cFpWUnZZWE4wWlhJaUxDSkdhWEp0ZDJGeVpWSmxkbWx6YVc5dUlqb3hMQ0pHYVhKdGQyRnlaVWx0WVdkbFUwaEJOVEV5SWpvaWFDOUtka293TlRFd1YwMU9RMXAyYWxkUFF6QlRVV292VWs5RVpUTXJTSE5GYTIwMGJVZE9lbFZNZEhCaFpWUnFkalpLV1N0M2NqUjVVMjVXZG5sR05UQmlRM0l4ZEhCeGQwTnZRV2htY0hGQ1dtd3lZbEU5UFNJc0lrVjRjR1ZqZEdWa1JtbHliWGRoY21WTlpXRnpkWEpsYldWdWRDSTZJbWd2U25aS01EVXhNRmROVGtOYWRtcFhUME13VTFGcUwxSlBSR1V6SzBoelJXdHRORzFIVG5wVlRIUndZV1ZVYW5ZMlNsa3JkM0kwZVZOdVZuWjVSalV3WWtOeU1YUndjWGREYjBGb1puQnhRbHBzTW1KUlBUMGlMQ0pDZFdsc1pGUnBiV1Z6ZEdGdGNDSTZJakl3TWpBdE1URXRNalJVTVRFNk5UZzZNRGxhSW4wPSIsIlNpZ25hdHVyZSI6IlRFOU1JUT09In0=","Checkpoint":{"TreeSize":94,"RootHash":"uVxL27F8Cn//fR540AOXYUAytI0Zzy3q6EsuIY6W+mM=","TimestampNanos":1606219089169612444},"InclusionProof":{"Value":null,"LeafIndex":93,"Proof":["Kv2X4qnhywOdFuygRzEfrp9r+1+6+oLeW0DqaFTl2ZQ=","EJWrh9mfnUYB2k8jdhLH47qAELCSBn993oBrQ2i/I1Q=","D3+q/Q0ZYUtWz1vTr90bmaZoF/Q/VJsScGiAYGG3M9g=","jAJirzErZ93Fa9qi2UCJ++6ygGY4X+jDRcukr0dmt0U=","+IVLLpscVZdckOLOnobkEx99wkHFIC1vhvIiWzFqkDM="]}}}`
	goldenFirmwareImage = `Firmware image`
)

func TestBundleForUpdate(t *testing.T) {
	var up api.UpdatePackage
	if err := json.Unmarshal([]byte(goldenUpdate), &up); err != nil {
		t.Fatalf(err.Error())
	}

	for _, test := range []struct {
		desc    string
		img     []byte
		wantErr bool
	}{
		{
			desc: "all good",
			img:  []byte(goldenFirmwareImage),
		}, {
			desc:    "bad image hash",
			img:     []byte("this is wrong"),
			wantErr: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			imgHash := sha512.Sum512(test.img)
			err := verify.BundleForUpdate(up.ProofBundle, imgHash[:])
			if (err != nil) != test.wantErr {
				t.Fatalf("want err %T, got %q", test.wantErr, err)
			}
		})
	}
}

func TestBundleForBoot(t *testing.T) {
	var up api.UpdatePackage
	if err := json.Unmarshal([]byte(goldenUpdate), &up); err != nil {
		t.Fatalf(err.Error())
	}
	imgHash := sha512.Sum512([]byte(goldenFirmwareImage))

	for _, test := range []struct {
		desc        string
		measurement []byte
		wantErr     bool
	}{
		{
			desc: "all good",
			// TODO(al): test measurement being different to H(img)
			measurement: imgHash[:],
		}, {
			desc:        "bad image hash",
			measurement: []byte("this is wrong"),
			wantErr:     true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			err := verify.BundleForBoot(up.ProofBundle, test.measurement)
			if (err != nil) != test.wantErr {
				t.Fatalf("want err %T, got %q", test.wantErr, err)
			}
		})
	}
}
