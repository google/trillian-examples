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
package common

import (
	"testing"

	"github.com/golang/glog"
)

func TestSignatureRoundTrip(t *testing.T) {
	for _, test := range []struct {
		desc       string
		signbody   string
		verifbody  string
		wantStatus bool
	}{
		{
			desc:       "Successful Signature Verification",
			signbody:   "My Test Message",
			verifbody:  "My Test Message",
			wantStatus: true,
		}, {
			desc:       "Unsuccessful Signature Verification",
			signbody:   "My Test Message",
			verifbody:  "My Test1 Message",
			wantStatus: false,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			msg := []byte(test.signbody)
			sign, err := SignMessage(msg)
			if err != nil {
				glog.Exitf("Failed to marshal statement: %v", err)
			}

			// Now Verify the signature
			msg = []byte(test.verifbody)
			ok, err := VerifySignature(msg, sign)
			switch {
			case err != nil && ok != test.wantStatus:
				t.Fatalf("Got unexpected error %q", err)
			case err == nil && ok != test.wantStatus:
				t.Fatal("Got no error, but wanted error")
			case err != nil && test.wantStatus:
				// expected error
			default:
				//Fall through
			}
		})
	}
}
