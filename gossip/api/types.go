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

// Package api describes the external interface to the gossip hub.
package api

import (
	"crypto/sha256"
	"fmt"

	"github.com/google/certificate-transparency-go/tls"
)

// PathPrefix is the common prefix for all HTTP entrypoints.
const PathPrefix = "/gossip/v0/"

// Add Signed Blob to Hub
// POST https://<hub server>/gossip/v0/add-signed-blob
// Inputs:
//    source_id: A unique identifier for the public key used to sign the blob.
//    blob_data: The binary blob
//    source_signature: The source's signature over blob_data.  This may be
//                      either Sign(Hash(blob_data)) or Sign(blob_data) depending
//                      on the source.Digest value.
// Outputs:
//    timestamped_entry: TLS-encoded TimestampedEntry.
//    hub_signature: hub's signature gossip_timestamped_entry
// Note that the returned timestamped_entry may have a different source_signature than
// was submitted (if the signature scheme is non-deterministic and an earlier submission
// of the same blob had a different signature).

// AddSignedBlobPath is the final path component for this entrypoint.
const AddSignedBlobPath = "add-signed-blob"

// AddSignedBlobRequest represents the JSON request body sent to the add-signed-blob POST method.
type AddSignedBlobRequest struct {
	SourceID        string `json:"source_id"`
	BlobData        []byte `json:"blob_data"`
	SourceSignature []byte `json:"src_signature"`
}

// AddSignedBlobResponse represents the JSON response body for the add-signed-blob POST method.
type AddSignedBlobResponse struct {
	// TimestampedEntryData holds the TLS-encoding of a TimestampedEntry.
	TimestampedEntryData []byte `json:"timestamped_entry"`
	// HubSignature is a signature over GossipTimestamp.
	HubSignature []byte `json:"hub_signature"`
}

// TimestampedEntry holds a timestamped entry in the hub, in a form suitable for TLS-encoding.
// RFC 5246 notation:
//   struct {
//      opaque source_id<1,65535>;
//      opaque blob_data<1,65535>;
//      opaque source_signature<0,65535>;
//      uint64 hub_timestamp;
//   } TimestampedEntry;
type TimestampedEntry struct {
	SourceID        []byte `tls:"minlen:1,maxlen:65535"`
	BlobData        []byte `tls:"minlen:1,maxlen:65535"`
	SourceSignature []byte `tls:"minlen:0,maxlen:65535"`
	HubTimestamp    uint64 // in nanoseconds since UNIX epoch
}

// SignedGossipTimestamp holds a timestamped entry in the gossip hub together with a
// signature from the hub.  RFC 5246 notation:
//   struct {
//      TimestampedEntry timestamped_entry;
//      opaque hub_signature<1,65535>;
//   } SignedGossipTimestamp;
type SignedGossipTimestamp struct {
	TimestampedEntry TimestampedEntry
	// HubSignature is over the TLS-encoding of GossipTimestamp.
	HubSignature []byte `tls:"minlen:1,maxlen:65535"`
}

// Retrieve Latest Signed Tree Head for Hub
// GET https://<hub server>/gossip/v0/get-sth
// Inputs: None
// Outputs:
//    head_data: TLS-encoded HubTreeHead.
//    hub_signature: signature over head_data.

// GetSTHPath is the final path component for this entrypoint.
const GetSTHPath = "get-sth"

// GetSTHResponse represents the JSON response to the get-sth GET method.
type GetSTHResponse struct {
	// HeadData holds the TLS-encoding of a HubTreeHead.
	TreeHeadData []byte `json:"head_data"`
	// HubSignature is a signature over TreeHeadData from the hub.
	HubSignature []byte `json:"hub_signature"`
}

// HubTreeHead describes a head of the Hub's Merkle tree. RFC 5246 notation:
//   struct {
//     uint64 tree_size;
//     uint64 timestamp;
//     opaque root_hash<1,65535>;
//   } HubTreeHead;
// TODO(drysdale): shift this to being a versioned structure to allow for
// future expansion (cf. github.com/google/trillian/types.LogRoot).
type HubTreeHead struct {
	TreeSize  uint64
	Timestamp uint64 // in nanoseconds since UNIX epoch
	RootHash  []byte `tls:"minlen:1,maxlen:65535"`
}

// SignedHubTreeHead describes a head of the Hub's Merkle tree, together with
// a signature from the hub.
type SignedHubTreeHead struct {
	TreeHead     HubTreeHead
	HubSignature []byte // over tls.Marshal(TreeHead)
}

// Retrieve Consistency Proof between Tree Heads
// GET https://<hub server>/gossip/v0/get-sth-consistency
// Inputs:
//    first:  The tree_size of the first tree, in decimal.
//    second:  The tree_size of the second tree, in decimal.
// Both tree sizes must be from existing v1 STHs (Signed Tree Heads).
// Outputs:
//    consistency:  An array of Merkle Tree nodes, base64 encoded.

// GetSTHConsistencyPath is the final path component for this entrypoint.
const GetSTHConsistencyPath = "get-sth-consistency"
const (
	// GetSTHConsistencyFirst is the first parameter name.
	GetSTHConsistencyFirst = "first"
	// GetSTHConsistencySecond is the second parameter name.
	GetSTHConsistencySecond = "second"
)

// GetSTHConsistencyResponse represents the JSON response to the get-sth-consistency GET method.
type GetSTHConsistencyResponse struct {
	Consistency [][]byte `json:"consistency"`
}

// Retrieve Inclusion Proof from Hub by entry hash
// GET https://<hub server>/gossip/v0/get-proof-by-hash
// Inputs:
//    hash:  A base64-encoded leaf hash, which is the leaf hash of a TLS-encoded
//           TimestampedEntry.
//    tree_size:  The tree_size of the tree on which to base the proof in deciaml.
// Outputs:
//    leaf_index:  The 0-based index of the entry corresponding to the hash parameter.
//    audit_path:  An array of Merkle Tree node, base64 encoded.

// GetProofByHashPath is the final path component for this entrypoint.
const GetProofByHashPath = "get-proof-by-hash"
const (
	// GetProofByHashArg is the first parameter name.
	GetProofByHashArg = "hash"
	// GetProofByHashSize is the second parameter name.
	GetProofByHashSize = "tree_size"
)

// GetProofByHashResponse represents the JSON response to the get-proof-by-hash GET method.
type GetProofByHashResponse struct {
	LeafIndex int64    `json:"leaf_index"`
	AuditPath [][]byte `json:"audit_path"`
}

// Retrieve Entries from Hub
// GET https://<hub server>/gossip/v0/get-entries
// Inputs:
//    start:  0-based index of first entry to retrieve, in decimal.
//    end:  0-based index of last entry to retrieve, in decimal.
// Outputs:
//    entries:  An array of objects, each of which is a TLS-encoded TimestampedEntry structure.
// TODO(drysdale): make this paginated

// GetEntriesPath is the final path component for this entrypoint.
const GetEntriesPath = "get-entries"

const (
	// GetEntriesStart is the first parameter name.
	GetEntriesStart = "start"
	// GetEntriesEnd is the second parameter name.
	GetEntriesEnd = "end"
)

// GetEntriesResponse respresents the JSON response to the get-entries GET method.
type GetEntriesResponse struct {
	// Entries holds TLS-encoded TimestampedEntry structures
	Entries [][]byte `json:"entries"` // the list of returned entries
}

// Retrieve Accepted Source Public Keys
// GET https://<hub server>/gossip/v0/get-source-keys
// Inputs: None
// Outputs:
//   entries: An array of objects, each consisting of:
//      id: The identifier for the source.
//      pub_key:  The base64-encoded public key for the source.
//      kind: The type of the source (e.g. an RFC 6962 CT Log generating
//            signed tree heads).

// GetSourceKeysPath is the final path component for this entrypoint.
const GetSourceKeysPath = "get-source-keys"

// GetSourceKeysResponse represents the JSON response to the get-source-keys GET method.
type GetSourceKeysResponse struct {
	Entries []*SourceKey `json:"entries"`
}

// Possible values for SourceKey.Kind, indicating the structure of signed blob
// expected to be submitted for this source.
const (
	// UnknownKind indicates that no information is known about the source of
	// the signed blobs (and so only signature verification is possible). Any
	// unrecognized kind values will be treated as UnknownKind.
	UnknownKind = "UNKNOWN"
	// RFC6962STHKind indicates that the source is expected to only generate
	// submissions that are Signed Tree Heads as described by RFC 6962.
	// The BlobData for a submission is expected to be the result of
	// tls.Marshal(ct.TreeHeadSignature), and may be rejected if this is not
	// the case.
	RFC6962STHKind = "RFC6962STH"
	// TrillianSLRKind indicates that the source is expected to generate
	// submissions that are Signed Log Roots from the Trillian Log API.
	// The BlobData for a submission is expected to be the result of
	// tls.Marshal(types.LogRoot), and may be rejected if this is not the
	// case.
	TrillianSLRKind = "TRILLIANSLR"
)

// SourceKey holds key information about a source that is tracked by this hub.
type SourceKey struct {
	ID     string `json:"id"`
	PubKey []byte `json:"pub_key"` // DER encoded
	Kind   string `json:"kind"`
	// Digest indicates that submissions for this source only contain
	// the hash digest of the source data, not the source data itself.
	Digest bool
}

// Retrieve latest entry for a source.
// The definition of 'latest' depends on what kind the source is; in general, it is the
// most recently submitted entry, but for a source of CT STHs, it is the entry with the
// latest timestamp inside the parsed STH data.
// GET https://<hub server>/gossip/v0/get-latest-for-src
// Inputs:
//    source_id: The source to retrieve latest data for.
// Outputs:
//    entry:  A TLS-encoded TimestampedEntry structure, holding the 'latest' known entry
//            for the source.  This is best-effort, and may not be available (in which
//            case a 204 No Content status is returned).

// GetLatestForSourcePath is the final path component for this entrypoint.
const GetLatestForSourcePath = "get-latest-for-src"

const (
	// GetLatestForSourceID is the parameter name.
	GetLatestForSourceID = "src_id"
)

// GetLatestForSourceResponse represents the JSON response to the get-proof-by-hash GET method.
type GetLatestForSourceResponse struct {
	Entry []byte `json:"entry"` // a TLS-encoded TimestampedEntry
}

// TimestampedEntryHash calculates the leaf hash value for a timestamped entry in the hub:
//   SHA256(0x00 || tls-encode(entry))
func TimestampedEntryHash(entry *TimestampedEntry) ([]byte, error) {
	data, err := tls.Marshal(*entry)
	if err != nil {
		return nil, fmt.Errorf("failed to tls.Marshal: %v", err)
	}
	h := sha256.New()
	h.Write([]byte{0x00})
	h.Write(data)
	return h.Sum(nil), nil
}
