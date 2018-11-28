// Copyright 2018 Google Inc. All Rights Reserved.
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

package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestTimestampedEntryHash(t *testing.T) {
	tests := []struct {
		desc    string
		in      TimestampedEntry
		mid     string // hex-encoded
		wantErr bool
	}{
		{
			desc:    "Empty",
			mid:     "0000" + "0000" + "0000" + "0000000000000000",
			wantErr: true,
		},
		{
			desc: "Valid",
			in: TimestampedEntry{
				SourceID:        []byte{0x20, 0x20},
				BlobData:        []byte{0x01, 0x02},
				SourceSignature: []byte{0x03, 0x04},
				HubTimestamp:    0x01020304,
			},
			mid: ("0002" + "2020") + ("0002" + "0102") + ("0002" + "0304") + ("0000000001020304"),
		},
		{
			desc: "NoData",
			in: TimestampedEntry{
				SourceID:        []byte{0x20, 0x20},
				SourceSignature: []byte{0x03, 0x04},
				HubTimestamp:    0x01020304,
			},
			mid:     ("0002" + "2020") + ("0000") + ("0002" + "0304") + ("0000000001020304"),
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			got, gotErr := TimestampedEntryHash(&test.in)
			if gotErr != nil {
				if !test.wantErr {
					t.Errorf("TimestampedEntryHash(%+v)=nil,%v, want _,nil", test.in, gotErr)
				}
				return
			}
			if test.wantErr {
				t.Errorf("TimestampedEntryHash(%+v)=%x,nil, want nil, non-nil", test.in, got)
			}
			midData, err := hex.DecodeString("00" + test.mid)
			if err != nil {
				t.Fatalf("Failed to decode test data: %v", err)
			}
			want := sha256.Sum256(midData)
			if !bytes.Equal(got, want[:]) {
				t.Errorf("TimestampedEntryHash(%+v)=%x, want %x", test.in, got, want)
			}
		})
	}
}
