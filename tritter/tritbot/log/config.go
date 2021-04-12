// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package log

import (
	tc "github.com/google/trillian/client"
	"github.com/google/trillian/merkle/rfc6962/hasher"
)

// TreeVerifier returns a verifier configured for the log.
func TreeVerifier() (*tc.LogVerifier, error) {
	return tc.NewLogVerifier(hasher.DefaultHasher), nil
}
