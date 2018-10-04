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

// Package integration holds test-only code for running tests on
// an integrated system of the Gossip Hub personality and a Trillian log.
package integration

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"reflect"
	"strings"
	"time"

	cryptorand "crypto/rand"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/certificate-transparency-go/jsonclient"
	"github.com/google/trillian-examples/gossip/api"
	"github.com/google/trillian-examples/gossip/client"
	"github.com/google/trillian-examples/gossip/hub/configpb"
	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/kylelemons/godebug/pretty"
)

var (
	// DefaultTransport is a http Transport more suited for use in a test context.
	// In particular it increases the number of reusable connections to the same
	// host. This helps to prevent starvation of ports through TIME_WAIT when
	// testing with a high number of parallel chain submissions.
	DefaultTransport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          1000,
		MaxIdleConnsPerHost:   1000,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Verifier is used to verify Merkle tree calculations.
	Verifier = merkle.NewLogVerifier(rfc6962.DefaultHasher)
)

// ClientPool describes an entity which produces HubClient instances.
type ClientPool interface {
	// Next returns the next HubClient instance to be used.
	Next() *client.HubClient
}

// RandomPool holds a collection of HubClient instances.
type RandomPool []*client.HubClient

// Next picks a random client from the pool.
func (p RandomPool) Next() *client.HubClient {
	if len(p) == 0 {
		return nil
	}
	return p[rand.Intn(len(p))]
}

// NewRandomPool creates a pool which returns a random client from a list of servers.
func NewRandomPool(servers string, pubKey *keyspb.PublicKey, prefix string) (ClientPool, error) {
	opts := jsonclient.Options{PublicKeyDER: pubKey.GetDer()}
	hc := &http.Client{Transport: DefaultTransport}

	var pool RandomPool
	for _, s := range strings.Split(servers, ",") {
		c, err := client.New(fmt.Sprintf("http://%s/%s", s, prefix), hc, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to create HubClient instance: %v", err)
		}
		pool = append(pool, c)
	}
	return &pool, nil
}

// signedBlob holds a blob of data that has been signed by a source.
type signedBlob api.AddSignedBlobRequest

// testInfo holds per-test information.
type testInfo struct {
	prefix  string
	cfg     *configpb.HubConfig
	pool    ClientPool
	srcKeys []*ecdsa.PrivateKey
}

func (t *testInfo) client() *client.HubClient {
	return t.pool.Next()
}

// RunIntegrationForHub tests against the Hub with configuration cfg, with a set
// of comma-separated server addresses given by servers.  The provided srcKeys
// are used to generate test content as from the sources, and so need to match
// the public keys in cfg.
// nolint: gocyclo
func RunIntegrationForHub(ctx context.Context, cfg *configpb.HubConfig, servers string, mmd time.Duration, srcKeys []*ecdsa.PrivateKey) error {
	if len(cfg.Source) != len(srcKeys) {
		return fmt.Errorf("source mismatch: %d configured, %d private keys provided", len(cfg.Source), len(srcKeys))
	}
	pool, err := NewRandomPool(servers, cfg.PublicKey, cfg.Prefix)
	if err != nil {
		return fmt.Errorf("failed to create pool: %v", err)
	}
	t := testInfo{
		prefix:  cfg.Prefix,
		cfg:     cfg,
		pool:    pool,
		srcKeys: srcKeys,
	}

	// Stage 0: get accepted source keys, which should match the private keys we have to play with.
	roots, err := t.client().GetSourceKeys(ctx)
	if err != nil {
		return fmt.Errorf("got GetSourceKeys()=(nil,%v); want (_,nil)", err)
	}
	fmt.Printf("%s: Accepts signed heads from: \n", t.prefix)
	got := make(map[string][]byte)
	for _, root := range roots {
		fmt.Printf("    %s: pubKey %s\n", root.ID, base64.StdEncoding.EncodeToString(root.PubKey))
		got[root.ID] = root.PubKey
	}
	want := make(map[string][]byte)
	for _, src := range t.cfg.Source {
		want[src.Id] = src.PublicKey.Der
	}
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("got GetSourceKeys()=%v; want %v", got, want)
	}

	// Stage 1: get the STH, which should be empty.
	sth0, err := t.client().GetSTH(ctx)
	if err != nil {
		return fmt.Errorf("got GetSTH()=(nil,%v); want (_,nil)", err)
	}
	if sth0.TreeHead.TreeSize != 0 {
		return fmt.Errorf("sth.TreeSize=%d; want 0", sth0.TreeHead.TreeSize)
	}
	fmt.Printf("%s: Got STH(time=%q, size=%d): roothash=%x\n", t.prefix, timeFromNS(sth0.TreeHead.Timestamp), sth0.TreeHead.TreeSize, sth0.TreeHead.RootHash)

	// Record every submission along the way.
	var signedBlobs []*signedBlob
	var sgts []*api.SignedGossipTimestamp

	// Stage 2: add a single signed blob, get an SGT.
	blob, err := t.getSignedBlob()
	if err != nil {
		return fmt.Errorf("failed to generate signed test blob: %v", err)
	}
	signedBlobs = append(signedBlobs, blob)

	sgt, err := t.client().AddSignedBlob(ctx, blob.SourceID, blob.BlobData, blob.SourceSignature)
	if err != nil {
		return fmt.Errorf("got AddSignedBlob()=(nil,%v); want (_,nil)", err)
	}
	sgts = append(sgts, sgt)
	// Display the SGT.
	fmt.Printf("%s: Uploaded blob from %s to Hub, got SGT(time=%q)\n", t.prefix, blob.SourceID, timeFromNS(sgt.TimestampedEntry.HubTimestamp))

	// Keep getting the STH until tree size becomes 1 and check the cert is included.
	sth1, err := t.awaitTreeSize(ctx, 1, true, mmd)
	if err != nil {
		return fmt.Errorf("got AwaitTreeSize(1)=(nil,%v); want (_,nil)", err)
	}
	fmt.Printf("%s: Got STH(time=%q, size=%d): roothash=%x\n", t.prefix, timeFromNS(sth1.TreeHead.Timestamp), sth1.TreeHead.TreeSize, sth1.TreeHead.RootHash)
	t.checkInclusionOf(ctx, blob, sgt, sth1)

	// Stage 3: add the same signed blob but with a different source signature,
	// expect an SGT that is the same as before (except the signature maybe).
	newSignature, err := t.regenerateSignature(blob)
	if err != nil {
		return fmt.Errorf("failed to generate new signature for blob: %v", err)
	}

	sgt, err = t.client().AddSignedBlob(ctx, blob.SourceID, blob.BlobData, newSignature)
	if err != nil {
		return fmt.Errorf("got re-AddSignedBlob()=(nil,%v); want (_,nil)", err)
	}
	if got, want := sgt.TimestampedEntry, sgts[0].TimestampedEntry; !reflect.DeepEqual(got, want) {
		return fmt.Errorf("got sgt %+v; want %+v", got, want)
	}

	// Stage 4: add a second blob, wait for tree size = 2
	blob, err = t.getSignedBlob()
	if err != nil {
		return fmt.Errorf("failed to generate signed test blob: %v", err)
	}
	signedBlobs = append(signedBlobs, blob)
	sgt, err = t.client().AddSignedBlob(ctx, blob.SourceID, blob.BlobData, blob.SourceSignature)
	if err != nil {
		return fmt.Errorf("got AddSignedBlob()=(nil,%v); want (_,nil)", err)
	}
	sgts = append(sgts, sgt)
	fmt.Printf("%s: Uploaded blob 2 from %s to Hub, got SGT(time=%q)\n", t.prefix, blob.SourceID, timeFromNS(sgt.TimestampedEntry.HubTimestamp))
	sth2, err := t.awaitTreeSize(ctx, 2, true, mmd)
	if err != nil {
		return fmt.Errorf("failed to get STH for size=1: %v", err)
	}
	fmt.Printf("%s: Got STH(time=%q, size=%d): roothash=%x\n", t.prefix, timeFromNS(sth2.TreeHead.Timestamp), sth2.TreeHead.TreeSize, sth2.TreeHead.RootHash)

	// Stage 5: get a consistency proof from size 1-> size 2.
	proof12, err := t.client().GetSTHConsistency(ctx, 1, 2)
	if err != nil {
		return fmt.Errorf("got GetSTHConsistency(1, 2)=(nil,%v); want (_,nil)", err)
	}
	//                 sth2
	//                 / \
	//  sth1   =>      a b
	//    |            | |
	//   d0           d0 d1
	// So consistency proof is [b] and we should have:
	//   sth2 == SHA256(0x01 | sth1 | b)
	if len(proof12) != 1 {
		return fmt.Errorf("len(proof12)=%d; want 1", len(proof12))
	}
	if err := checkConsistencyProof(sth1, sth2, proof12); err != nil {
		return fmt.Errorf("got checkConsistencyProof(sth1,sth2,proof12)=%v; want nil", err)
	}

	// Stage 6: get a consistency proof from size 0-> size 2, which should be empty.
	proof02, err := t.client().GetSTHConsistency(ctx, 0, 2)
	if err != nil {
		return fmt.Errorf("got GetSTHConsistency(0, 2)=(nil,%v); want (_,nil)", err)
	}
	if len(proof02) != 0 {
		return fmt.Errorf("len(proof02)=%d; want 0", len(proof02))
	}

	// Stage 7: add blobs [2], [3], [4], [5],...[N], for some random N.
	const maxBlobs = 21
	const atLeast = 4
	n := atLeast + rand.Intn(maxBlobs-1-atLeast)
	for i := 2; i <= n; i++ {
		blob, err = t.getSignedBlob()
		if err != nil {
			return fmt.Errorf("failed to generate signed test blob %d: %v", i, err)
		}
		signedBlobs = append(signedBlobs, blob)

		sgt, err = t.client().AddSignedBlob(ctx, blob.SourceID, blob.BlobData, blob.SourceSignature)
		if err != nil {
			return fmt.Errorf("got AddSignedBlob(blob %d)=(nil,%v); want (_,nil)", i, err)
		}
		sgts = append(sgts, sgt)
	}
	fmt.Printf("%s: Uploaded blob02-blob%02d to Hub, got SGTs\n", t.prefix, n)

	// Stage 8: keep getting the STH until tree size becomes N+1.
	treeSize := n + 1
	sthN, err := t.awaitTreeSize(ctx, uint64(treeSize), true, mmd)
	if err != nil {
		return fmt.Errorf("got AwaitTreeSize(%d)=(nil,%v); want (_,nil)", treeSize, err)
	}
	fmt.Printf("%s: Got STH(time=%q, size=%d): roothash=%x\n", t.prefix, timeFromNS(sthN.TreeHead.Timestamp), sthN.TreeHead.TreeSize, sthN.TreeHead.RootHash)

	// Stage 9: get a consistency proof from 2->(N+1).
	proof2N, err := t.client().GetSTHConsistency(ctx, 2, uint64(treeSize))
	if err != nil {
		return fmt.Errorf("got GetSTHConsistency(2, %d)=(nil,%v); want (_,nil)", treeSize, err)
	}
	fmt.Printf("%s: Proof size 2->%d: %x\n", t.prefix, treeSize, proof2N)
	if err := checkConsistencyProof(sth2, sthN, proof2N); err != nil {
		return fmt.Errorf("got CheckCTConsistencyProof(sth2,sthN,proof2N)=%v; want nil", err)
	}

	// Stage 10: get entries [0, N] (start at 1 to skip int-ca.cert)
	entries, err := t.client().GetEntries(ctx, 0, int64(treeSize))
	if err != nil {
		return fmt.Errorf("got GetEntries(0,%d)=(nil,%v); want (_,nil)", treeSize, err)
	}
	if len(entries) < treeSize {
		return fmt.Errorf("len(entries)=%d; want %d", len(entries), treeSize)
	}
	// The blobs might not be sequenced in the order they were uploaded, so compare
	// a set of hashes.
	gotHashes := make(map[[sha256.Size]byte]bool)
	for _, entry := range entries {
		gotHashes[sha256.Sum256(entry.BlobData)] = true
	}
	wantHashes := make(map[[sha256.Size]byte]bool)
	for i := 0; i < treeSize; i++ {
		wantHashes[sha256.Sum256(signedBlobs[i].BlobData)] = true
	}

	if diff := pretty.Compare(gotHashes, wantHashes); diff != "" {
		return fmt.Errorf("retrieved cert hashes don't match uploaded cert hashes, diff:\n%v", diff)
	}
	fmt.Printf("%s: Got entries [0:%d)\n", t.prefix, treeSize)

	// Stage 11: get an audit proof for each signed blob that we have an SGT for.
	for i := 0; i < treeSize; i++ {
		t.checkInclusionOf(ctx, signedBlobs[i], sgts[i], sthN)
	}
	fmt.Printf("%s: Got inclusion proofs [0:%d)\n", t.prefix, treeSize)

	// Stage 12: attempt to upload a blob with an incorrect signature.
	corruptBlob := *signedBlobs[0]
	corruptAt := len(corruptBlob.SourceSignature) - 2
	corruptBlob.SourceSignature[corruptAt] = corruptBlob.SourceSignature[corruptAt] + 1
	sgt, err = t.client().AddSignedBlob(ctx, corruptBlob.SourceID, corruptBlob.BlobData, corruptBlob.SourceSignature)
	if err == nil {
		return fmt.Errorf("got AddChain(corrupt-sig)=(%+v,nil); want (nil,error)", sgt)
	}
	fmt.Printf("%s: AddChain(corrupt-sig)=nil,%v\n", t.prefix, err)

	// Stage 13: invalid consistency proof
	proof, err := t.client().GetSTHConsistency(ctx, 2, 299)
	if err == nil {
		return fmt.Errorf("got GetSTHConsistency(2,299)=(%x,nil); want (nil,_)", proof)
	}
	fmt.Printf("%s: GetSTHConsistency(2,299)=(nil,%v)\n", t.prefix, err)

	// Stage 14: invalid inclusion proof; expect a client.RspError{404}.
	wrong := sha256.Sum256([]byte("simply wrong"))
	rsp, err := t.client().GetProofByHash(ctx, wrong[:], sthN.TreeHead.TreeSize)
	if err == nil {
		return fmt.Errorf("got GetProofByHash(wrong, size=%d)=(%v,nil); want (nil,_)", sthN.TreeHead.TreeSize, rsp)
	} else if rspErr, ok := err.(jsonclient.RspError); ok {
		if rspErr.StatusCode != http.StatusNotFound {
			return fmt.Errorf("got GetProofByHash(wrong)=_, %d; want (nil, 404)", rspErr.StatusCode)
		}
	} else {
		return fmt.Errorf("got GetProofByHash(wrong)=%+v (%T); want (client.RspError)", err, err)
	}
	fmt.Printf("%s: GetProofByHash(wrong,%d)=(nil,%v)\n", t.prefix, sthN.TreeHead.TreeSize, err)

	return nil
}

// timeFromNS converts a timestamp in nanoseconds since UNIX epoch to a time.Time.
func timeFromNS(ts uint64) time.Time {
	return time.Unix(0, int64(ts))
}

func (t *testInfo) getSignedBlob() (*signedBlob, error) {
	which := rand.Intn(len(t.cfg.Source))
	src := t.cfg.Source[which]
	privKey := t.srcKeys[which]

	data := make([]byte, 100)
	rand.Read(data)

	digest := sha256.Sum256(data)
	sig, err := privKey.Sign(cryptorand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("failed to sign blob data: %v", err)
	}

	return &signedBlob{
		SourceID:        src.Id,
		BlobData:        data,
		SourceSignature: sig,
	}, nil
}
func (t *testInfo) regenerateSignature(blob *signedBlob) ([]byte, error) {
	var privKey *ecdsa.PrivateKey
	for i, src := range t.cfg.Source {
		if src.Id == blob.SourceID {
			privKey = t.srcKeys[i]
			break
		}
	}
	if privKey == nil {
		return nil, fmt.Errorf("failed to find private key for %s", blob.SourceID)
	}

	digest := sha256.Sum256(blob.BlobData)
	sig, err := privKey.Sign(cryptorand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("failed to sign blob data: %v", err)
	}
	return sig, nil
}

// awaitTreeSize loops until an STH is retrieved that is the specified size (or larger, if exact is false).
func (t *testInfo) awaitTreeSize(ctx context.Context, size uint64, exact bool, mmd time.Duration) (*api.SignedHubTreeHead, error) {
	var sth *api.SignedHubTreeHead
	deadline := time.Now().Add(mmd)
	for sth == nil || sth.TreeHead.TreeSize < size {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("deadline for STH inclusion expired (MMD=%v)", mmd)
		}
		time.Sleep(200 * time.Millisecond)
		var err error
		sth, err = t.client().GetSTH(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get STH: %v", err)
		}
	}
	if exact && sth.TreeHead.TreeSize != size {
		return nil, fmt.Errorf("sth.TreeSize=%d; want: %d", sth.TreeHead.TreeSize, size)
	}
	return sth, nil
}

// checkInclusionOf checks that a given signed blob and assocated SCT are included
// under a signed tree head.
func (t *testInfo) checkInclusionOf(ctx context.Context, signedBlob *signedBlob, sgt *api.SignedGossipTimestamp, sth *api.SignedHubTreeHead) error {
	// First check that the fields match between the submitted blob and the SGT.
	if got, want := sgt.TimestampedEntry.SourceID, []byte(signedBlob.SourceID); !bytes.Equal(got, want) {
		return fmt.Errorf("mismatch in source ID between SGT (%q) and submitted blob (%q)", got, want)
	}
	if got, want := sgt.TimestampedEntry.BlobData, signedBlob.BlobData; !bytes.Equal(got, want) {
		return fmt.Errorf("mismatch in head data between SGT (%x) and submitted blob (%x)", got, want)
	}
	if got, want := sgt.TimestampedEntry.SourceSignature, signedBlob.SourceSignature; !bytes.Equal(got, want) {
		return fmt.Errorf("mismatch in source signature between SGT (%x) and submitted blob (%x)", got, want)
	}
	entryHash, err := api.TimestampedEntryHash(&sgt.TimestampedEntry)
	if err != nil {
		return fmt.Errorf("failed to calculate timestamped entry hash: %v", err)
	}

	rsp, err := t.client().GetProofByHash(ctx, entryHash[:], sth.TreeHead.TreeSize)
	if err != nil {
		return fmt.Errorf("got GetProofByHash(sct[%d],size=%d)=(nil,%v); want (_,nil)", 0, sth.TreeHead.TreeSize, err)
	}
	if err := Verifier.VerifyInclusionProof(rsp.LeafIndex, int64(sth.TreeHead.TreeSize), rsp.AuditPath, sth.TreeHead.RootHash, entryHash); err != nil {
		return fmt.Errorf("got VerifyInclusionProof(%d, %d,...)=%v", 0, sth.TreeHead.TreeSize, err)
	}
	return nil
}

// checkConsistencyProof checks the given consistency proof.
func checkConsistencyProof(sth1, sth2 *api.SignedHubTreeHead, proof [][]byte) error {
	return Verifier.VerifyConsistencyProof(int64(sth1.TreeHead.TreeSize), int64(sth2.TreeHead.TreeSize),
		sth1.TreeHead.RootHash, sth2.TreeHead.RootHash, proof)
}

// BuildTestConfig builds a test Hub configuration for the specified number
// of Gossip Hubs, each of which tracks the srcCount sources.  The private
// keys for the sources are also returned, to allow creation and submission
// of source-signed data.
func BuildTestConfig(hubCount, srcCount int) ([]*configpb.HubConfig, []*ecdsa.PrivateKey, error) {
	var srcKeys []*ecdsa.PrivateKey
	var srcs []*configpb.TrackedSource
	for i := 0; i < srcCount; i++ {
		// Create a source private key which allows us to generate loggable content.
		srcKey, err := ecdsa.GenerateKey(elliptic.P256(), cryptorand.Reader)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate ECDSA key for source: %v", err)
		}
		pubKeyDER, err := x509.MarshalPKIXPublicKey(srcKey.Public())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal source public key to DER: %v", err)
		}
		src := configpb.TrackedSource{
			Name:          fmt.Sprintf("source-%02d", i),
			Id:            fmt.Sprintf("https://source-%02d.example.com", i),
			PublicKey:     &keyspb.PublicKey{Der: pubKeyDER},
			HashAlgorithm: sigpb.DigitallySigned_SHA256,
		}
		srcKeys = append(srcKeys, srcKey)
		srcs = append(srcs, &src)
	}

	var cfgs []*configpb.HubConfig
	for i := 0; i < hubCount; i++ {
		// Create a per-Hub private key
		hubKey, err := ecdsa.GenerateKey(elliptic.P256(), cryptorand.Reader)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate ECDSA key for Hub: %v", err)
		}
		hubKeyDER, err := der.MarshalPrivateKey(hubKey)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate private key DER for Hub: %v", err)
		}
		hubKeyProto, err := ptypes.MarshalAny(&keyspb.PrivateKey{Der: hubKeyDER})
		if err != nil {
			return nil, nil, fmt.Errorf("fould not marshal Hub private key as protobuf Any: %v", err)
		}
		pubKeyDER, err := x509.MarshalPKIXPublicKey(hubKey.Public())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal public key to DER: %v", err)
		}
		hubCfg := configpb.HubConfig{
			Prefix:     fmt.Sprintf("hub-tone-%02d", i),
			Source:     srcs,
			PrivateKey: hubKeyProto,
			PublicKey:  &keyspb.PublicKey{Der: pubKeyDER},
		}
		cfgs = append(cfgs, &hubCfg)
	}
	return cfgs, srcKeys, nil
}
