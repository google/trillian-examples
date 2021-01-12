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

package verify

import (
	"github.com/google/trillian/merkle/logverifier"
	"github.com/google/trillian/merkle/rfc6962/hasher"
)

// NewLogVerifier is a convenience function to return a log verifier suitable for use
// with the FT personality.
func NewLogVerifier() logverifier.LogVerifier {
	return logverifier.New(hasher.DefaultHasher)
}

// HashLeaf is a convenience function to return a leaf hash for a given leaf.
func HashLeaf(leaf []byte) []byte {
	return hasher.DefaultHasher.HashLeaf(leaf)
}
