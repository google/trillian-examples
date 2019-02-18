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
	"github.com/google/certificate-transparency-go/tls"
	"github.com/google/trillian-examples/gossip/api"
	"github.com/google/trillian-examples/gossip/client"
	"github.com/google/trillian-examples/gossip/hub/configpb"
	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/types"
	"github.com/kylelemons/godebug/pretty"

	ct "github.com/google/certificate-transparency-go"
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
	prefix     string
	cfg        *configpb.HubConfig
	pool       ClientPool
	srcKeys    []*ecdsa.PrivateKey
	srcsOfKind map[configpb.TrackedSource_Kind][]int
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

	// Track which kinds of sources have been configured.
	srcsOfKind := make(map[configpb.TrackedSource_Kind][]int)
	for i, src := range cfg.Source {
		srcsOfKind[src.Kind] = append(srcsOfKind[src.Kind], i)
	}

	pool, err := NewRandomPool(servers, cfg.PublicKey, cfg.Prefix)
	if err != nil {
		return fmt.Errorf("failed to create pool: %v", err)
	}
	t := testInfo{
		prefix:     cfg.Prefix,
		cfg:        cfg,
		pool:       pool,
		srcKeys:    srcKeys,
		srcsOfKind: srcsOfKind,
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
	newBlob, err := t.reSignedBlob(blob)
	if err != nil {
		return fmt.Errorf("failed to generate new signature for blob: %v", err)
	}

	sgt, err = t.client().AddSignedBlob(ctx, newBlob.SourceID, newBlob.BlobData, newBlob.SourceSignature)
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
		return fmt.Errorf("got consistencyProof(sth2,sthN,proof2N)=%v; want nil", err)
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
		return fmt.Errorf("got AddSignedBlob(corrupt-sig)=(%+v,nil); want (nil,error)", sgt)
	}
	fmt.Printf("%s: AddSignedBlob(corrupt-sig)=nil,%v\n", t.prefix, err)

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

	if src, privKey := t.pickSourceOfKind(configpb.TrackedSource_RFC6962STH); src != nil && privKey != nil {
		// [CT-specific] tests.

		// Stage 15: add a blob signed by a CT Log key that isn't an STH.
		data := make([]byte, 100)
		rand.Read(data)
		nonBlob, err := blobForData(src, privKey, data)
		if err != nil {
			return fmt.Errorf("failed to sign blob data: %v", err)
		}
		sgt, err = t.client().AddSignedBlob(ctx, nonBlob.SourceID, nonBlob.BlobData, nonBlob.SourceSignature)
		if err == nil {
			return fmt.Errorf("got AddSignedBlob(non-STH-blob)=(%+v,nil); want (nil,error)", sgt)
		}
		fmt.Printf("%s: AddSignedBlob(non-STH-blob)=nil,%v\n", t.prefix, err)

		// Stage 16: add a bigger STH and check get-latest-for-src shows it.
		// Use the same client throughout, so best-effort get-latest-for-src works.
		client := t.client()
		sth1Blob, err := blobForData(src, privKey, dataForSTH(200, 1000005))
		if err != nil {
			return fmt.Errorf("failed to generate STH test blob: %v", err)
		}
		sth1SGT, err := client.AddSignedBlob(ctx, sth1Blob.SourceID, sth1Blob.BlobData, sth1Blob.SourceSignature)
		if err != nil {
			return fmt.Errorf("got AddSignedBlob(sth-200-blob)=(nil,%v); want (_,nil)", err)
		}
		fmt.Printf("%s: Uploaded sth-200-blob to hub, got SGT\n", t.prefix)

		entry, err := client.GetLatestForSource(ctx, sth1Blob.SourceID)
		if err != nil {
			return fmt.Errorf("got GetLatestForSource(%s)=(nil,%v); want (_,nil)", sth1Blob.SourceID, err)
		}
		if !reflect.DeepEqual(entry, &sth1SGT.TimestampedEntry) {
			return fmt.Errorf("got GetLatestForSource(%s)=%+v; want %+v", sth1Blob.SourceID, entry, sth1SGT)
		}
		fmt.Printf("%s: GetLatestForSource(%s) matches sth-200-blob\n", t.prefix, sth1Blob.SourceID)

		// Stage 17: add a smaller STH and check get-latest-for-src is unaffected.
		sth2Blob, err := blobForData(src, privKey, dataForSTH(150, 1000002))
		if err != nil {
			return fmt.Errorf("failed to generate STH test blob: %v", err)
		}
		if _, err := client.AddSignedBlob(ctx, sth2Blob.SourceID, sth2Blob.BlobData, sth2Blob.SourceSignature); err != nil {
			return fmt.Errorf("got AddSignedBlob(sth-150-blob)=(nil,%v); want (_,nil)", err)
		}
		fmt.Printf("%s: Uploaded sth-150-blob to hub, got SGT\n", t.prefix)

		entry, err = client.GetLatestForSource(ctx, sth2Blob.SourceID)
		if err != nil {
			return fmt.Errorf("got GetLatestForSource(%s)=(nil,%v); want (_,nil)", sth1Blob.SourceID, err)
		}
		if !reflect.DeepEqual(entry, &sth1SGT.TimestampedEntry) {
			return fmt.Errorf("got GetLatestForSource(%s)=%+v; want %+v", sth1Blob.SourceID, entry, sth1SGT)
		}
		fmt.Printf("%s: GetLatestForSource(%s) still matches sth-200-blob\n", t.prefix, sth1Blob.SourceID)
	}

	if src, privKey := t.pickSourceOfKind(configpb.TrackedSource_TRILLIANSLR); src != nil && privKey != nil {
		// [Trillian SLR-specific] tests.

		// Stage 15: add a blob signed by a Trillian Log key that isn't an SLR.
		data := make([]byte, 100)
		rand.Read(data)
		nonBlob, err := blobForData(src, privKey, data)
		if err != nil {
			return fmt.Errorf("failed to sign blob data: %v", err)
		}
		sgt, err = t.client().AddSignedBlob(ctx, nonBlob.SourceID, nonBlob.BlobData, nonBlob.SourceSignature)
		if err == nil {
			return fmt.Errorf("got AddSignedBlob(non-SLR-blob)=(%+v,nil); want (nil,error)", sgt)
		}
		fmt.Printf("%s: AddSignedBlob(non-SLR-blob)=nil,%v\n", t.prefix, err)

		// Stage 16: add a bigger SLR and check get-latest-for-src shows it.
		// Use the same client throughout, so best-effort get-latest-for-src works.
		client := t.client()
		slr1Blob, err := blobForData(src, privKey, dataForSLR(200, 1000005))
		if err != nil {
			return fmt.Errorf("failed to generate SLR test blob: %v", err)
		}
		slr1SGT, err := client.AddSignedBlob(ctx, slr1Blob.SourceID, slr1Blob.BlobData, slr1Blob.SourceSignature)
		if err != nil {
			return fmt.Errorf("got AddSignedBlob(slr-200-blob)=(nil,%v); want (_,nil)", err)
		}
		fmt.Printf("%s: Uploaded slr-200-blob to hub, got SGT\n", t.prefix)

		entry, err := client.GetLatestForSource(ctx, slr1Blob.SourceID)
		if err != nil {
			return fmt.Errorf("got GetLatestForSource(%s)=(nil,%v); want (_,nil)", slr1Blob.SourceID, err)
		}
		if !reflect.DeepEqual(entry, &slr1SGT.TimestampedEntry) {
			return fmt.Errorf("got GetLatestForSource(%s)=%+v; want %+v", slr1Blob.SourceID, entry, slr1SGT)
		}
		fmt.Printf("%s: GetLatestForSource(%s) matches slr-200-blob\n", t.prefix, slr1Blob.SourceID)

		// Stage 17: add a smaller SLR and check get-latest-for-src is unaffected.
		slr2Blob, err := blobForData(src, privKey, dataForSLR(150, 1000002))
		if err != nil {
			return fmt.Errorf("failed to generate SLR test blob: %v", err)
		}
		if _, err := client.AddSignedBlob(ctx, slr2Blob.SourceID, slr2Blob.BlobData, slr2Blob.SourceSignature); err != nil {
			return fmt.Errorf("got AddSignedBlob(slr-150-blob)=(nil,%v); want (_,nil)", err)
		}
		fmt.Printf("%s: Uploaded slr-150-blob to hub, got SGT\n", t.prefix)

		entry, err = client.GetLatestForSource(ctx, slr2Blob.SourceID)
		if err != nil {
			return fmt.Errorf("got GetLatestForSource(%s)=(nil,%v); want (_,nil)", slr1Blob.SourceID, err)
		}
		if !reflect.DeepEqual(entry, &slr1SGT.TimestampedEntry) {
			return fmt.Errorf("got GetLatestForSource(%s)=%+v; want %+v", slr1Blob.SourceID, entry, slr1SGT)
		}
		fmt.Printf("%s: GetLatestForSource(%s) still matches slr-200-blob\n", t.prefix, slr1Blob.SourceID)
	}

	if src, privKey := t.pickSourceOfKind(configpb.TrackedSource_TRILLIANSMR); src != nil && privKey != nil {
		// [Trillian SMR-specific] tests.

		// Stage 15: add a blob signed by a Trillian Map key that isn't an SMR.
		data := make([]byte, 100)
		rand.Read(data)
		nonBlob, err := blobForData(src, privKey, data)
		if err != nil {
			return fmt.Errorf("failed to sign blob data: %v", err)
		}
		sgt, err = t.client().AddSignedBlob(ctx, nonBlob.SourceID, nonBlob.BlobData, nonBlob.SourceSignature)
		if err == nil {
			return fmt.Errorf("got AddSignedBlob(non-SMR-blob)=(%+v,nil); want (nil,error)", sgt)
		}
		fmt.Printf("%s: AddSignedBlob(non-SMR-blob)=nil,%v\n", t.prefix, err)

		// Stage 16: add a bigger SMR and check get-latest-for-src shows it.
		// Use the same client throughout, so best-effort get-latest-for-src works.
		client := t.client()
		smr1Blob, err := blobForData(src, privKey, dataForSMR(200, 1000005))
		if err != nil {
			return fmt.Errorf("failed to generate SMR test blob: %v", err)
		}
		smr1SGT, err := client.AddSignedBlob(ctx, smr1Blob.SourceID, smr1Blob.BlobData, smr1Blob.SourceSignature)
		if err != nil {
			return fmt.Errorf("got AddSignedBlob(smr-200-blob)=(nil,%v); want (_,nil)", err)
		}
		fmt.Printf("%s: Uploaded smr-200-blob to hub, got SGT\n", t.prefix)

		entry, err := client.GetLatestForSource(ctx, smr1Blob.SourceID)
		if err != nil {
			return fmt.Errorf("got GetLatestForSource(%s)=(nil,%v); want (_,nil)", smr1Blob.SourceID, err)
		}
		if !reflect.DeepEqual(entry, &smr1SGT.TimestampedEntry) {
			return fmt.Errorf("got GetLatestForSource(%s)=%+v; want %+v", smr1Blob.SourceID, entry, smr1SGT)
		}
		fmt.Printf("%s: GetLatestForSource(%s) matches smr-200-blob\n", t.prefix, smr1Blob.SourceID)

		// Stage 17: add a smaller SMR and check get-latest-for-src is unaffected.
		smr2Blob, err := blobForData(src, privKey, dataForSMR(150, 1000002))
		if err != nil {
			return fmt.Errorf("failed to generate SMR test blob: %v", err)
		}
		if _, err := client.AddSignedBlob(ctx, smr2Blob.SourceID, smr2Blob.BlobData, smr2Blob.SourceSignature); err != nil {
			return fmt.Errorf("got AddSignedBlob(smr-150-blob)=(nil,%v); want (_,nil)", err)
		}
		fmt.Printf("%s: Uploaded smr-150-blob to hub, got SGT\n", t.prefix)

		entry, err = client.GetLatestForSource(ctx, smr2Blob.SourceID)
		if err != nil {
			return fmt.Errorf("got GetLatestForSource(%s)=(nil,%v); want (_,nil)", smr1Blob.SourceID, err)
		}
		if !reflect.DeepEqual(entry, &smr1SGT.TimestampedEntry) {
			return fmt.Errorf("got GetLatestForSource(%s)=%+v; want %+v", smr1Blob.SourceID, entry, smr1SGT)
		}
		fmt.Printf("%s: GetLatestForSource(%s) still matches smr-200-blob\n", t.prefix, smr1Blob.SourceID)
	}

	return nil
}

// timeFromNS converts a timestamp in nanoseconds since UNIX epoch to a time.Time.
func timeFromNS(ts uint64) time.Time {
	return time.Unix(0, int64(ts))
}

func (t *testInfo) getSignedBlob() (*signedBlob, error) {
	src, privKey := t.pickSource()
	return t.blobForSource(src, privKey)
}

func (t *testInfo) pickSource() (*configpb.TrackedSource, *ecdsa.PrivateKey) {
	which := rand.Intn(len(t.cfg.Source))
	return t.cfg.Source[which], t.srcKeys[which]
}

func (t *testInfo) pickSourceOfKind(kind configpb.TrackedSource_Kind) (*configpb.TrackedSource, *ecdsa.PrivateKey) {
	srcIndices := t.srcsOfKind[kind]
	if len(srcIndices) == 0 {
		return nil, nil
	}
	idx := rand.Intn(len(srcIndices))
	which := srcIndices[idx]
	return t.cfg.Source[which], t.srcKeys[which]
}

func dataForSTH(sz, ts uint64) []byte {
	th := ct.TreeHeadSignature{
		Version:       ct.V1,
		SignatureType: ct.TreeHashSignatureType,
		TreeSize:      sz,
		Timestamp:     ts,
	}
	rand.Read(th.SHA256RootHash[:])
	data, err := tls.Marshal(th)
	if err != nil {
		panic(fmt.Sprintf("failed to create fake CT tree head: %v", err))
	}
	return data
}

func dataForSLR(sz, ts uint64) []byte {
	lr := types.LogRoot{
		Version: 1,
		V1: &types.LogRootV1{
			TreeSize:       sz,
			TimestampNanos: ts * 1000 * 1000,
			RootHash:       make([]byte, sha256.Size),
		},
	}
	rand.Read(lr.V1.RootHash)
	data, err := tls.Marshal(lr)
	if err != nil {
		panic(fmt.Sprintf("failed to create fake Trillian LogRoot: %v", err))
	}
	return data
}

func dataForSMR(rev, ts uint64) []byte {
	mr := types.MapRoot{
		Version: 1,
		V1: &types.MapRootV1{
			Revision:       rev,
			TimestampNanos: ts * 1000 * 1000,
			RootHash:       make([]byte, sha256.Size),
		},
	}
	rand.Read(mr.V1.RootHash)
	data, err := tls.Marshal(mr)
	if err != nil {
		panic(fmt.Sprintf("failed to create fake Trillian MapRoot: %v", err))
	}
	return data
}

func (t *testInfo) blobForSource(src *configpb.TrackedSource, privKey *ecdsa.PrivateKey) (*signedBlob, error) {
	var data []byte
	switch src.Kind {
	case configpb.TrackedSource_RFC6962STH:
		data = dataForSTH(100, 1000000)
	case configpb.TrackedSource_TRILLIANSLR:
		data = dataForSLR(100, 1000000)
	case configpb.TrackedSource_TRILLIANSMR:
		data = dataForSMR(100, 1000000)
	default:
		data = make([]byte, 100)
		rand.Read(data)
	}
	return blobForData(src, privKey, data)
}

func blobForData(src *configpb.TrackedSource, privKey *ecdsa.PrivateKey, data []byte) (*signedBlob, error) {
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

func (t *testInfo) reSignedBlob(blob *signedBlob) (*signedBlob, error) {
	for i, src := range t.cfg.Source {
		if src.Id == blob.SourceID {
			privKey := t.srcKeys[i]
			return blobForData(src, privKey, blob.BlobData)
		}
	}
	return nil, fmt.Errorf("failed to find private key for %s", blob.SourceID)
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
// of source-signed data.  The created sources alternate between generators
// of CT STHs and generators of anonymous signed blobs.
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
		kind := configpb.TrackedSource_UNKNOWN
		switch i % 4 {
		case 0:
			kind = configpb.TrackedSource_RFC6962STH
		case 1:
			kind = configpb.TrackedSource_TRILLIANSLR
		case 2:
			kind = configpb.TrackedSource_TRILLIANSMR
		}
		src := configpb.TrackedSource{
			Name:          fmt.Sprintf("source-%02d", i),
			Id:            fmt.Sprintf("https://source-%02d.example.com", i),
			PublicKey:     &keyspb.PublicKey{Der: pubKeyDER},
			HashAlgorithm: sigpb.DigitallySigned_SHA256,
			Kind:          kind,
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
