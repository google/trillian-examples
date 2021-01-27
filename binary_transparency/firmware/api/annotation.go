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

package api

// Enum values for the different types of statement.
const (
	FirmwareMetadataType    byte = 'f'
	MalwareStatementType    byte = 'm'
	RevocationStatementType byte = 'r'
)

// SignedStatement is a Statement signed by the Claimant.
type SignedStatement struct {
	// Type is one of the statement types from above, and indicates what
	// Statement should be interpreted as.
	Type byte
	// The serialised Claim in json form.
	// This is one of MalwareStatement or BuildStatement.
	// TODO(mhutchinson): Add an enum for Statement type?
	Statement []byte

	// Signature is the bytestream of the signature over (Type || Statement).
	Signature []byte
}

// FirmwareID is a pointer to a firmware version.
// It will be a SignedStatement of type FirmwareMetadataType.
// TODO(mhutchinson): This could be simplified to just LeafHash or extended to have Revision.
type FirmwareID struct {
	LogIndex int64
	LeafHash []byte
}

// MalwareStatement is an annotation about malware checks in a firmware version.
type MalwareStatement struct {
	// FirmwareID is the SignedStatement in the log being annotated.
	FirmwareID FirmwareID

	// Good is a crude signal of goodness.
	// TODO(mhutchinson): MVP for reasonable fields. Probably a Timestamp.
	Good bool
}

// RevocationStatement is an annotation that marks a build as revoked.
// This statement simply being present for a build marks it as revoked.
// There is no way to unrevoke something; this can be done by re-releasing it.
type RevocationStatement struct {
	// FirmwareID is the SignedStatement in the log being annotated.
	FirmwareID FirmwareID
}
