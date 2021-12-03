// Copyright 2021 Google LLC. All Rights Reserved.
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

package pixelbt

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"testing"
)

func TestConvertToNote(t *testing.T) {
	toB64 := base64.StdEncoding.EncodeToString

	for i, test := range []struct {
		name    string
		keyName string
		keyHash uint32
		cpTxt   string
		want    []byte
		wantErr bool
	}{
		{
			name:    "golden",
			keyName: "Pixel-Transparency-Log-2021",
			keyHash: 0x72c878db,
			cpTxt:   "DEFAULT\n16\nJeLAbi5nGYiJBrVNYwQAPuTl4eJtdh0/v2xGQ+mQBRo=\n\nMEYCIQDdsdzgRvnn/z3UTDwoFdLvJagBUPWvyrqPoakKJoifBAIhAJO/xFa/rh6cDHNS8XgJFM3zvSTJbp4bDZfzR00I6uXi\n",
			want:    []byte("DEFAULT\n16\nJeLAbi5nGYiJBrVNYwQAPuTl4eJtdh0/v2xGQ+mQBRo=\n\n— Pixel-Transparency-Log-2021 csh42zBGAiEA3bHc4Eb55/891Ew8KBXS7yWoAVD1r8q6j6GpCiaInwQCIQCTv8RWv64enAxzUvF4CRTN870kyW6eGw2X80dNCOrl4g==\n"),
		}, {
			name:    "works",
			keyName: "Key",
			keyHash: 0x12345678,
			cpTxt:   "bananas\n" + "\n" + toB64([]byte("signature")),
			want:    []byte("bananas\n\n— Key " + base64.StdEncoding.EncodeToString(append([]byte{0x12, 0x34, 0x56, 0x78}, []byte("signature")...)) + "\n"),
		}, {
			name:    "works too",
			keyName: "Key-Too",
			keyHash: 0x98765432,
			cpTxt:   "lemons\n" + "\n" + toB64([]byte("hancock")),
			want:    []byte("lemons\n\n— Key-Too " + base64.StdEncoding.EncodeToString(append([]byte{0x98, 0x76, 0x54, 0x32}, []byte("hancock")...)) + "\n"),
		}, {
			name:    "missing newline on body",
			keyName: "Key",
			keyHash: 0x98765432,
			cpTxt:   "lemons" + "\n" + toB64([]byte("hancock")),
			wantErr: true,
		}, {
			name:    "invalid chars in keyName",
			keyName: "Key+Name",
			keyHash: 0x98765432,
			wantErr: true,
		},
	} {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			got, err := convertToNote(test.cpTxt, test.keyName, test.keyHash)
			if gotErr := err != nil; gotErr != test.wantErr {
				t.Fatalf("ToNote = %v, want no error", err)
			}
			if !bytes.Equal(got, test.want) {
				t.Fatalf("ToNote =\n%q\nwant\n%q", got, test.want)
			}
		})
	}
}
