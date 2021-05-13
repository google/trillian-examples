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

package format

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// Proof represents a common format proof.
//
// Interpretation of the proof bytes is ecosystem dependent.
type Proof [][]byte

// Marshal returns the common format representation of this proof.
func (p Proof) Marshal() string {
	b := strings.Builder{}
	for _, l := range p {
		b.WriteString(base64.StdEncoding.EncodeToString(l))
		b.WriteRune('\n')
	}
	return b.String()
}

// Unmarshal parses common proof format data and stores the result in the
// Proof struct.
func (p *Proof) Unmarshal(data []byte) error {
	const delim = "\n"
	lines := strings.Split(strings.TrimRight(string(data), delim), delim)
	r := make([][]byte, len(lines))
	for i, l := range lines {
		b, err := base64.StdEncoding.DecodeString(l)
		if err != nil {
			return fmt.Errorf("failed to decode proof line %d: %w", i, err)
		}
		r[i] = b
	}
	(*p) = r
	return nil
}
