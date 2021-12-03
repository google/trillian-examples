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
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// convertToNote converts a the PixelBT checkpoint to a valid Note.
//
// Hopefully we won't need this for too long, and PixelBT will update their checkpoint format to make it
// fully compatible.
func convertToNote(pixelCP, keyName string, keyHash uint32) ([]byte, error) {
	split := strings.LastIndex(pixelCP, "\n\n")
	if split < 0 {
		return nil, fmt.Errorf("invalid Pixel CP %q", pixelCP)
	}
	cpBody, sigB64 := pixelCP[:split+1], pixelCP[split+2:]

	if !strings.HasSuffix(cpBody, "\n") {
		return nil, errors.New("checkpoint body does not end with a newline")
	}
	validName := keyName != "" && utf8.ValidString(keyName) && strings.IndexFunc(keyName, unicode.IsSpace) < 0 && !strings.Contains(keyName, "+")
	if !validName {
		return nil, fmt.Errorf("invalid keyname %q", keyName)
	}
	if len(sigB64) == 0 {
		return nil, errors.New("invalid checkpoint - empty signature")
	}
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 in signature %q: %v", sigB64, err)
	}

	var buf bytes.Buffer
	buf.WriteString(cpBody)
	buf.WriteString("\n")

	var hbuf [4]byte
	binary.BigEndian.PutUint32(hbuf[:], keyHash)
	b64 := base64.StdEncoding.EncodeToString(append(hbuf[:], []byte(sig)...))
	buf.WriteString("— ")
	buf.WriteString(keyName)
	buf.WriteString(" ")
	buf.WriteString(b64)
	buf.WriteString("\n")
	return buf.Bytes(), nil
}
