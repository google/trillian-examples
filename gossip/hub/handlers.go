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

package hub

import (
	"context"
	"crypto"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/certificate-transparency-go/tls"
	"github.com/google/trillian"
	"github.com/google/trillian-examples/gossip/api"
	"github.com/google/trillian-examples/gossip/hub/configpb"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ct "github.com/google/certificate-transparency-go"
	tcrypto "github.com/google/trillian/crypto"
)

const (
	contentTypeHeader     = "Content-Type"
	contentTypeJSON       = "application/json"
	defaultMaxGetEntries  = int64(1000)
	sourceQuotaUserPrefix = "@source"
)

var (
	// Use an explicitly empty slice for empty proofs so it gets JSON-encoded as
	// '[]' rather than 'null'.
	emptyProof = make([][]byte, 0)
)

var (
	// Metrics are all per-hub (label "hubid"), but may also be
	// per-entrypoint (label "ep") or per-return-code (label "rc").
	once             sync.Once
	knownHubs        monitoring.Gauge     // hubid => value (always 1.0)
	lastSTHTimestamp monitoring.Gauge     // hubid => value
	lastSTHTreeSize  monitoring.Gauge     // hubid => value
	reqsCounter      monitoring.Counter   // hubid, ep => value
	rspsCounter      monitoring.Counter   // hubid, ep, rc => value
	rspLatency       monitoring.Histogram // hubid, ep, rc => value
)

// setupMetrics initializes all the exported metrics.
func setupMetrics(mf monitoring.MetricFactory) {
	knownHubs = mf.NewGauge("known_hubs", "Set to 1 for known hubs", "hubid")
	lastSTHTimestamp = mf.NewGauge("last_sth_timestamp", "Time of last STH in ms since epoch", "hubid")
	lastSTHTreeSize = mf.NewGauge("last_sth_treesize", "Size of tree at last STH", "hubid")
	reqsCounter = mf.NewCounter("http_reqs", "Number of requests", "hubid", "ep")
	rspsCounter = mf.NewCounter("http_rsps", "Number of responses", "hubid", "ep", "rc")
	rspLatency = mf.NewHistogram("http_latency", "Latency of responses in seconds", "hubid", "ep", "rc")
}

// Entrypoints is a list of entrypoint names as exposed in statistics/logging.
var Entrypoints = []string{api.AddSignedBlobPath, api.GetSTHPath, api.GetSTHConsistencyPath, api.GetProofByHashPath, api.GetEntriesPath, api.GetSourceKeysPath, api.GetLatestForSourcePath}

// kindHandler holds handlers for processing that is specific to particular
// kinds of sources.
type kindHandler struct {
	// submissionCheck performs a check on submitted data to see if it should be admitted.
	submissionCheck func(data, sig []byte) error
	// entryIsLater indicates whether one entry for this kind of source is considered
	// to be 'later' than another.
	entryIsLater func(prev *api.TimestampedEntry, current *api.TimestampedEntry) bool
}

var kindHandlers = map[configpb.TrackedSource_Kind]kindHandler{
	configpb.TrackedSource_RFC6962STH:  {submissionCheck: ctSTHChecker, entryIsLater: ctSTHIsLater},
	configpb.TrackedSource_TRILLIANSLR: {submissionCheck: trillianSLRChecker, entryIsLater: trillianSLRIsLater},
}

func ctSTHChecker(data, sig []byte) error {
	// Check that the data has the structure of a CT tree head.
	var th ct.TreeHeadSignature
	if rest, err := tls.Unmarshal(data, &th); err != nil {
		return fmt.Errorf("submission for CT source failed to parse as tree head: %v", err)
	} else if len(rest) > 0 {
		return fmt.Errorf("submission for CT source has %d bytes of trailing data after tree head", len(rest))
	}
	return nil
}

func trillianSLRChecker(data, sig []byte) error {
	// Check that the data has the structure of a Trillian LogRoot
	var lr types.LogRoot
	if rest, err := tls.Unmarshal(data, &lr); err != nil {
		return fmt.Errorf("submission for Trillian LogRoot source failed to parse as log root: %v", err)
	} else if len(rest) > 0 {
		return fmt.Errorf("submission for Trillian LogRoot source has %d bytes of trailing data after log root", len(rest))
	} else if lr.Version != 1 {
		return fmt.Errorf("submission for Trillian LogRoot source has unknown version %d", lr.Version)
	}
	return nil
}

// ctSTHIsLater indicates whether the current entry is later than the prev entry,
// according to the tree size and timestamp held in the CT Log STH.
func ctSTHIsLater(prev *api.TimestampedEntry, current *api.TimestampedEntry) bool {
	var prevTH ct.TreeHeadSignature
	if _, err := tls.Unmarshal(prev.BlobData, &prevTH); err != nil {
		return true
	}
	var curTH ct.TreeHeadSignature
	if _, err := tls.Unmarshal(current.BlobData, &curTH); err != nil {
		return false
	}
	if curTH.TreeSize > prevTH.TreeSize {
		return true
	}
	if curTH.TreeSize < prevTH.TreeSize {
		return false
	}
	return curTH.Timestamp > prevTH.Timestamp
}

func trillianSLRIsLater(prev *api.TimestampedEntry, current *api.TimestampedEntry) bool {
	var prevLR types.LogRoot
	if _, err := tls.Unmarshal(prev.BlobData, &prevLR); err != nil {
		return true
	}
	if prevLR.Version != 1 {
		return true
	}
	var curLR types.LogRoot
	if _, err := tls.Unmarshal(current.BlobData, &curLR); err != nil {
		return false
	}
	if curLR.Version != 1 {
		return false
	}
	if curLR.V1.TreeSize > prevLR.V1.TreeSize {
		return true
	}
	if curLR.V1.TreeSize < prevLR.V1.TreeSize {
		return false
	}
	return curLR.V1.TimestampNanos > prevLR.V1.TimestampNanos
}

// rawEntryIsLater indicates whether the current entry is later than the prev entry,
// based on the timestamp when the entry was originally submitted to the Hub.
func rawEntryIsLater(prev *api.TimestampedEntry, current *api.TimestampedEntry) bool {
	return current.HubTimestamp > prev.HubTimestamp
}

// PathHandlers maps from a path to the relevant AppHandler instance.
type PathHandlers map[string]AppHandler

// AppHandler holds a hubInfo and a handler function that uses it, and is
// an implementation of the http.Handler interface.
type AppHandler struct {
	Name    string
	info    *hubInfo
	handler func(context.Context, *hubInfo, http.ResponseWriter, *http.Request) (int, error)
	method  string // http.MethodGet or http.MethodPost
}

// ServeHTTP for an AppHandler invokes the underlying handler function but
// does additional common error and stats processing.
func (a AppHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	label0 := strconv.FormatInt(a.info.logID, 10)
	label1 := string(a.Name)
	reqsCounter.Inc(label0, label1)
	var status int
	startTime := time.Now()
	defer func() {
		rspLatency.Observe(time.Since(startTime).Seconds(), label0, label1, strconv.Itoa(status))
	}()
	glog.V(2).Infof("%s: request %v %q => %s", a.info.hubPrefix, r.Method, r.URL, a.Name)
	if r.Method != a.method {
		glog.Warningf("%s: %s wrong HTTP method: %v", a.info.hubPrefix, a.Name, r.Method)
		sendHTTPError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed: %s", r.Method))
		return
	}

	// For GET requests all params come as form encoded so we might as well parse them now.
	// POSTs will decode the raw request body as JSON later.
	if r.Method == http.MethodGet {
		if err := r.ParseForm(); err != nil {
			sendHTTPError(w, http.StatusBadRequest, fmt.Errorf("failed to parse form data: %v", err))
			return
		}
	}

	// Many/most of the handlers forward the request on to the Trillian Log RPC server;
	// impose a deadline on this onward request.
	ctx, cancel := context.WithTimeout(ctx, a.info.opts.Deadline)
	defer cancel()

	// Store any available chargeable-user info in the context.
	qctx := a.info.addCharge(ctx, r)

	status, err := a.handler(qctx, a.info, w, r)
	glog.V(2).Infof("%s: %s <= status=%d", a.info.hubPrefix, a.Name, status)
	rspsCounter.Inc(label0, label1, strconv.Itoa(status))
	if err != nil {
		glog.Warningf("%s: %s handler error: %v", a.info.hubPrefix, a.Name, err)
		sendHTTPError(w, status, err)
		return
	}

	// Additional check, for consistency the handler must return an error for non-200 status
	if status != http.StatusOK {
		glog.Warningf("%s: %s handler non 200 without error: %d %v", a.info.hubPrefix, a.Name, status, err)
		sendHTTPError(w, http.StatusInternalServerError, fmt.Errorf("http handler misbehaved, status: %d", status))
		return
	}
}

// sourceCryptoInfo holds information needed to check signatures generated by a source.
type sourceCryptoInfo struct {
	pubKeyData []byte // DER-encoded public key
	pubKey     crypto.PublicKey
	hasher     crypto.Hash
	kind       configpb.TrackedSource_Kind
	digest     bool
	verify     func(pub crypto.PublicKey, hasher crypto.Hash, digest, sig []byte) error
}

// hubInfo holds information about a specific hub instance.
type hubInfo struct {
	// Instance-wide options
	opts InstanceOptions

	hubPrefix string
	logID     int64
	urlPrefix string
	rpcClient trillian.TrillianLogClient
	signer    *tcrypto.Signer
	cryptoMap map[string]sourceCryptoInfo
	// latestEntry keeps a (local) record of the 'latest' entry for a source.
	// The definition of 'latest' varies according to what kind the source is.
	latestMu    sync.RWMutex
	latestEntry map[string]*api.TimestampedEntry
}

// newHubInfo creates a new instance of hubInfo.
func newHubInfo(logID int64, prefix string, rpcClient trillian.TrillianLogClient, signer crypto.Signer, cryptoMap map[string]sourceCryptoInfo, opts InstanceOptions) *hubInfo {
	info := &hubInfo{
		opts:        opts,
		hubPrefix:   fmt.Sprintf("%s{%d}", prefix, logID),
		logID:       logID,
		urlPrefix:   prefix,
		rpcClient:   rpcClient,
		signer:      tcrypto.NewSigner(logID, signer, crypto.SHA256),
		cryptoMap:   cryptoMap,
		latestEntry: make(map[string]*api.TimestampedEntry),
	}
	once.Do(func() { setupMetrics(opts.MetricFactory) })
	knownHubs.Set(1.0, strconv.FormatInt(logID, 10))

	return info
}

func (h *hubInfo) getLatestEntry(srcID string) *api.TimestampedEntry {
	h.latestMu.RLock()
	defer h.latestMu.RUnlock()
	return h.latestEntry[srcID]
}

func (h *hubInfo) setLatestEntry(srcID string, entry *api.TimestampedEntry) {
	h.latestMu.Lock()
	defer h.latestMu.Unlock()
	h.latestEntry[srcID] = entry
}

func buildLeaf(req *api.AddSignedBlobRequest, timeNanos uint64) (*trillian.LogLeaf, error) {
	// Build a TimestampedEntry for the data, initially skipping the timestamp and signature
	// in order to create a timeless identity hash.
	entry := api.TimestampedEntry{
		SourceID: []byte(req.SourceID),
		BlobData: req.BlobData,
	}

	timelessData, err := tls.Marshal(entry)
	if err != nil {
		return nil, fmt.Errorf("failed to create timeless hub leaf: %v", err)
	}
	identityHash := sha256.Sum256(timelessData)

	// The full leaf has a timestamp and signature (but there may be a pre-existing entry
	// with different values for these fields).
	entry.SourceSignature = req.SourceSignature
	entry.HubTimestamp = timeNanos
	leafData, err := tls.Marshal(entry)
	if err != nil {
		return nil, fmt.Errorf("failed to create timestamped hub leaf: %v", err)
	}

	return &trillian.LogLeaf{
		LeafValue:        leafData,
		LeafIdentityHash: identityHash[:],
	}, nil
}

func (h *hubInfo) addSignedBlob(ctx context.Context, apiReq *api.AddSignedBlobRequest) (*api.AddSignedBlobResponse, int, error) {
	// Find the source and its public key.
	cryptoInfo, ok := h.cryptoMap[apiReq.SourceID]
	if !ok {
		glog.V(1).Infof("%s: unknown source %q", h.hubPrefix, apiReq.SourceID)
		return nil, http.StatusNotFound, fmt.Errorf("unknown source %q", apiReq.SourceID)
	}

	// Verify the source's signature.
	if err := cryptoInfo.verify(cryptoInfo.pubKey, cryptoInfo.hasher, apiReq.BlobData, apiReq.SourceSignature); err != nil {
		glog.V(1).Infof("%s: failed to validate signature from %q: %v", h.hubPrefix, apiReq.SourceID, err)
		return nil, http.StatusBadRequest, fmt.Errorf("failed to validate signature from %q", apiReq.SourceID)
	}

	// Perform any other checks appropriate for the kind of source.
	if kc := kindHandlers[cryptoInfo.kind].submissionCheck; kc != nil {
		if err := kc(apiReq.BlobData, apiReq.SourceSignature); err != nil {
			glog.V(1).Infof("%s: failed to validate %v-specific data from %q: %v", h.hubPrefix, cryptoInfo.kind, apiReq.SourceID, err)
			return nil, http.StatusBadRequest, fmt.Errorf("failed to validate data from %q: %v", apiReq.SourceID, err)
		}
	}

	// Charge the operation to a per-source quota bucket, so that sources that sign a lot of things
	// (e.g. Trillian logs with fast STH generation) can't easily swamp the Hub.
	charge := appendCharge(chargeTo(ctx), fmt.Sprintf("%s %s", sourceQuotaUserPrefix, apiReq.SourceID))

	// Use the current time (in nanos since Unix epoch) and use to build the Trillian leaf.
	timeNanos := uint64(time.Now().UnixNano())
	leaf, err := buildLeaf(apiReq, timeNanos)
	if err != nil {
		glog.V(1).Infof("%s: failed to create leaf for %q: %v", h.hubPrefix, apiReq.SourceID, err)
		return nil, http.StatusBadRequest, fmt.Errorf("failed to create leaf: %v", err)
	}

	// Send the leaf on to the Trillian Log server.
	glog.V(2).Infof("%s: AddLogHead => grpc.QueueLeaves", h.hubPrefix)
	req := trillian.QueueLeavesRequest{
		LogId:    h.logID,
		Leaves:   []*trillian.LogLeaf{leaf},
		ChargeTo: charge,
	}
	rsp, err := h.rpcClient.QueueLeaves(ctx, &req)
	glog.V(2).Infof("%s: AddLogHead <= grpc.QueueLeaves err=%v", h.hubPrefix, err)
	if err != nil {
		return nil, h.toHTTPStatus(err), fmt.Errorf("backend QueueLeaves request failed: %v", err)
	}
	if rsp == nil {
		return nil, http.StatusInternalServerError, errors.New("missing QueueLeaves response")
	}
	if len(rsp.QueuedLeaves) != 1 {
		return nil, http.StatusInternalServerError, fmt.Errorf("unexpected QueueLeaves response leaf count: %d", len(rsp.QueuedLeaves))
	}
	entryData := rsp.QueuedLeaves[0].Leaf.LeafValue

	// Always use the data from returned leaf as the basis for an signed gossip
	// timestamp -- there may be an earlier logged entry, which may also have a
	// different signature than the submitted blob (for a non-deterministic
	// signature scheme like ECDSA).
	var loggedEntry api.TimestampedEntry
	if rest, err := tls.Unmarshal(entryData, &loggedEntry); err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to reconstruct TimestampedEntry: %s", err)
	} else if len(rest) > 0 {
		return nil, http.StatusInternalServerError, fmt.Errorf("extra data (%d bytes) on reconstructing TimestampedEntry", len(rest))
	}

	signature, err := h.signer.Sign(entryData)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to sign SGT data: %v", err)
	}

	// Update our local record of the 'latest' entry for this source, where 'latest' depends on the kind
	// of the source.  This is best-effort, so no need for atomic test-and-set.
	isLater := rawEntryIsLater
	if c := kindHandlers[cryptoInfo.kind].entryIsLater; c != nil {
		isLater = c
	}
	prevEntry := h.getLatestEntry(apiReq.SourceID)
	if prevEntry == nil || isLater(prevEntry, &loggedEntry) {
		h.setLatestEntry(apiReq.SourceID, &loggedEntry)
	}

	return &api.AddSignedBlobResponse{TimestampedEntryData: entryData, HubSignature: signature}, http.StatusOK, nil
}

// GetLogRoot retrieves a log root for the given Trillian log.
func GetLogRoot(ctx context.Context, client trillian.TrillianLogClient, logID int64, prefix string) (*types.LogRootV1, error) {
	// Send request to the Log server.
	req := trillian.GetLatestSignedLogRootRequest{
		LogId:    logID,
		ChargeTo: chargeTo(ctx),
	}
	glog.V(2).Infof("%s: GetLogRoot => grpc.GetLatestSignedLogRoot %+v", prefix, req)
	rsp, err := client.GetLatestSignedLogRoot(ctx, &req)
	glog.V(2).Infof("%s: GetLogRoot <= grpc.GetLatestSignedLogRoot err=%v", prefix, err)
	if err != nil {
		return nil, fmt.Errorf("backend GetLatestSignedLogRoot request failed: %v", err)
	}

	// Check over the response.
	slr := rsp.SignedLogRoot
	if slr == nil {
		return nil, errors.New("no log root returned")
	}
	// TODO(drysdale): configure the hub with the Trillian public key for this tree, and check the Trillian signature here.
	var root types.LogRootV1
	if err := root.UnmarshalBinary(slr.LogRoot); err != nil {
		return nil, fmt.Errorf("failed to unmarshal log root: %v", err)
	}

	glog.V(3).Infof("%s: GetLogRoot <= root=%+v", prefix, root)
	return &root, nil
}

func (h *hubInfo) getSTH(ctx context.Context) (*api.GetSTHResponse, int, error) {
	root, err := GetLogRoot(ctx, h.rpcClient, h.logID, h.hubPrefix)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}

	hth := api.HubTreeHead{
		TreeSize:  root.TreeSize,
		Timestamp: root.TimestampNanos, // in nanoseconds since UNIX epoch
		RootHash:  root.RootHash,
	}
	hthData, err := tls.Marshal(hth)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to marshal tree head: %v", err)
	}
	signature, err := h.signer.Sign(hthData)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to sign STH data: %v", err)
	}

	return &api.GetSTHResponse{TreeHeadData: hthData, HubSignature: signature}, http.StatusOK, nil
}

func (h *hubInfo) getSTHConsistency(ctx context.Context, first, second int64) (*api.GetSTHConsistencyResponse, int, error) {
	if first < 0 || second < 0 {
		return nil, http.StatusBadRequest, fmt.Errorf("first and second params cannot be <0: %d %d", first, second)
	}
	if second < first {
		return nil, http.StatusBadRequest, fmt.Errorf("invalid first, second params: %d %d", first, second)
	}

	var jsonRsp api.GetSTHConsistencyResponse
	if first != 0 {
		req := trillian.GetConsistencyProofRequest{
			LogId:          h.logID,
			FirstTreeSize:  first,
			SecondTreeSize: second,
			ChargeTo:       chargeTo(ctx),
		}

		glog.V(2).Infof("%s: GetSTHConsistency(%d, %d) => grpc.GetConsistencyProof %+v", h.hubPrefix, first, second, req)
		rsp, err := h.rpcClient.GetConsistencyProof(ctx, &req)
		glog.V(2).Infof("%s: GetSTHConsistency <= grpc.GetConsistencyProof err=%v", h.hubPrefix, err)
		if err != nil {
			return nil, h.toHTTPStatus(err), fmt.Errorf("backend GetConsistencyProof request failed: %v", err)
		}

		// TODO(drysdale): configure the hub with the Trillian public key for this tree, and check the Trillian signature here.
		var root types.LogRootV1
		if err := root.UnmarshalBinary(rsp.SignedLogRoot.LogRoot); err != nil {
			return nil, http.StatusInternalServerError, fmt.Errorf("failed to unmarshal log root: %v", err)
		}

		// We can get here with a tree size too small to satisfy the proof.
		if int64(root.TreeSize) < second {
			return nil, http.StatusBadRequest, fmt.Errorf("need tree size: %d for proof but only got: %d", second, root.TreeSize)
		}

		if err := checkHashSizes(rsp.Proof.Hashes); err != nil {
			return nil, http.StatusInternalServerError, fmt.Errorf("backend returned invalid proof %v: %v", rsp.Proof, err)
		}

		// We got a valid response from the server. Marshal it as JSON and return it to the client
		jsonRsp.Consistency = rsp.Proof.Hashes
		if jsonRsp.Consistency == nil {
			jsonRsp.Consistency = emptyProof
		}
	} else {
		glog.V(2).Infof("%s: GetSTHConsistency(%d, %d) starts from 0 so return empty proof", h.hubPrefix, first, second)
		jsonRsp.Consistency = emptyProof
	}
	return &jsonRsp, http.StatusOK, nil
}

func (h *hubInfo) getProofByHash(ctx context.Context, leafHash []byte, treeSize int64) (*api.GetProofByHashResponse, int, error) {
	req := trillian.GetInclusionProofByHashRequest{
		LogId:           h.logID,
		LeafHash:        leafHash,
		TreeSize:        treeSize,
		OrderBySequence: true,
		ChargeTo:        chargeTo(ctx),
	}
	rsp, err := h.rpcClient.GetInclusionProofByHash(ctx, &req)
	if err != nil {
		return nil, h.toHTTPStatus(err), fmt.Errorf("backend GetInclusionProofByHash request failed: %v", err)
	}

	// TODO(drysdale): configure the hub with the Trillian public key for this tree, and check the Trillian signature here.
	var root types.LogRootV1
	if err := root.UnmarshalBinary(rsp.SignedLogRoot.LogRoot); err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to unmarshal log root: %v", err)
	}

	// We could fail to get the proof because the tree size that the server has is not large enough.
	if int64(root.TreeSize) < treeSize {
		return nil, http.StatusNotFound, fmt.Errorf("log returned tree size: %d but we expected: %d", root.TreeSize, treeSize)
	}

	// Additional sanity checks on the response.
	if len(rsp.Proof) == 0 {
		// The backend returns the STH even when there is no proof, so explicitly map this to 4xx.
		return nil, http.StatusNotFound, errors.New("backend did not return a proof")
	}
	if err := checkHashSizes(rsp.Proof[0].Hashes); err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("backend returned invalid proof %v: %v", rsp.Proof, err)
	}

	// All checks complete, marshal and return the response
	jsonRsp := api.GetProofByHashResponse{
		LeafIndex: rsp.Proof[0].LeafIndex,
		AuditPath: rsp.Proof[0].Hashes,
	}
	if jsonRsp.AuditPath == nil {
		jsonRsp.AuditPath = emptyProof
	}
	return &jsonRsp, http.StatusOK, nil
}

func (h *hubInfo) getEntries(ctx context.Context, start, end int64) (*api.GetEntriesResponse, int, error) {
	// Make sure the parameters are sane to prevent an unnecessary round trip.
	if start < 0 || end < 0 {
		return nil, http.StatusBadRequest, fmt.Errorf("start (%d) and end (%d) parameters must be >= 0", start, end)
	}
	if start > end {
		return nil, http.StatusBadRequest, fmt.Errorf("start (%d) and end (%d) is not a valid range", start, end)
	}
	maxRange := h.opts.MaxGetEntries
	if maxRange == 0 {
		maxRange = defaultMaxGetEntries
	}

	count := end - start + 1
	if count > maxRange {
		end = start + maxRange - 1
		count = maxRange
	}

	// Now make a request to the backend to get the relevant leaves
	req := trillian.GetLeavesByRangeRequest{
		LogId:      h.logID,
		StartIndex: start,
		Count:      count,
		ChargeTo:   chargeTo(ctx),
	}
	rsp, err := h.rpcClient.GetLeavesByRange(ctx, &req)
	if err != nil {
		return nil, h.toHTTPStatus(err), fmt.Errorf("backend GetLeavesByRange request failed: %v", err)
	}
	// TODO(drysdale): configure the hub with the Trillian public key for this tree, and check the Trillian signature here.
	var root types.LogRootV1
	if err := root.UnmarshalBinary(rsp.SignedLogRoot.LogRoot); err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to unmarshal log root: %v", err)
	}
	if int64(root.TreeSize) <= start {
		// If the returned tree is too small to contain any leaves return the 4xx explicitly here.
		return nil, http.StatusBadRequest, fmt.Errorf("need tree size: %d to get leaves but only got: %d", root.TreeSize, start)
	}
	// Do some sanity checks on the result.
	if len(rsp.Leaves) > int(count) {
		return nil, http.StatusInternalServerError, fmt.Errorf("backend returned too many leaves: %d vs [%d,%d]", len(rsp.Leaves), start, end)
	}

	var jsonRsp api.GetEntriesResponse
	for i, leaf := range rsp.Leaves {
		if leaf.LeafIndex != start+int64(i) {
			return nil, http.StatusInternalServerError, fmt.Errorf("backend returned unexpected leaf index: rsp.Leaves[%d].LeafIndex=%d for range [%d,%d]", i, leaf.LeafIndex, start, end)
		}
		// Check that the data unmarshals OK before returning it.
		var entry api.TimestampedEntry
		if rest, err := tls.Unmarshal(leaf.LeafValue, &entry); err != nil {
			return nil, http.StatusInternalServerError, fmt.Errorf("%s: Failed to deserialize Merkle leaf from backend: %d", h.hubPrefix, leaf.LeafIndex)
		} else if len(rest) > 0 {
			return nil, http.StatusInternalServerError, fmt.Errorf("%s: Trailing data after Merkle leaf from backend: %d", h.hubPrefix, leaf.LeafIndex)
		}
		jsonRsp.Entries = append(jsonRsp.Entries, leaf.LeafValue)
	}
	return &jsonRsp, http.StatusOK, nil
}

func (h *hubInfo) getSourceKeys(ctx context.Context) *api.GetSourceKeysResponse {
	var jsonRsp api.GetSourceKeysResponse
	for sourceID, info := range h.cryptoMap {
		kind := api.UnknownKind
		switch info.kind {
		case configpb.TrackedSource_RFC6962STH:
			kind = api.RFC6962STHKind
		case configpb.TrackedSource_TRILLIANSLR:
			kind = api.TrillianSLRKind
		}
		l := api.SourceKey{ID: sourceID, PubKey: info.pubKeyData, Kind: kind, Digest: info.digest}
		jsonRsp.Entries = append(jsonRsp.Entries, &l)
	}
	return &jsonRsp
}

func (h *hubInfo) getLatestForSource(ctx context.Context, srcID string) (*api.GetLatestForSourceResponse, int, error) {
	if _, ok := h.cryptoMap[srcID]; !ok {
		glog.V(1).Infof("%s: unknown source %q", h.hubPrefix, srcID)
		return nil, http.StatusNotFound, fmt.Errorf("unknown source %q", srcID)
	}
	entry := h.getLatestEntry(srcID)
	if entry == nil {
		glog.V(1).Infof("%s: no latest entry for source %q", h.hubPrefix, srcID)
		return nil, http.StatusNoContent, fmt.Errorf("no latest entry for source %q", srcID)
	}
	entryData, err := tls.Marshal(*entry)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to serialize latest entry: %v", err)
	}
	return &api.GetLatestForSourceResponse{Entry: entryData}, http.StatusOK, nil
}

type quotaContextType string

// remoteQuotaCtxKey is the key used to attach a Trillian quota user to a context.
var remoteQuotaCtxKey = quotaContextType("quotaUser")

// addCharge creates a new context that includes user quota charging information
// if available.
func (h *hubInfo) addCharge(ctx context.Context, r *http.Request) context.Context {
	if h.opts.RemoteQuotaUser == nil {
		return ctx
	}
	return context.WithValue(ctx, remoteQuotaCtxKey, h.opts.RemoteQuotaUser(r))
}

// chargeTo retrieves information about which user quotas should be charged
// for a request from the context.
func chargeTo(ctx context.Context) *trillian.ChargeTo {
	q := ctx.Value(remoteQuotaCtxKey)
	if q == nil {
		return nil
	}
	qUser, ok := q.(string)
	if !ok {
		return nil
	}
	return &trillian.ChargeTo{User: []string{qUser}}
}

// appendCharge adds the specified user to the passed in ChargeTo and
// returns a new combined ChargeTo.
func appendCharge(a *trillian.ChargeTo, user string) *trillian.ChargeTo {
	return &trillian.ChargeTo{User: append(a.GetUser(), user)}
}

// Handlers returns a map from URL paths (with the given prefix) and AppHandler instances
// to handle those entrypoints.
func (h *hubInfo) Handlers(prefix string) PathHandlers {
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	prefix = strings.TrimRight(prefix, "/")

	// Bind the hubInfo instance to give an appHandler instance for each entrypoint.
	return PathHandlers{
		prefix + api.PathPrefix + api.AddSignedBlobPath:      AppHandler{info: h, handler: addSignedBlob, Name: api.AddSignedBlobPath, method: http.MethodPost},
		prefix + api.PathPrefix + api.GetSTHPath:             AppHandler{info: h, handler: getSTH, Name: api.GetSTHPath, method: http.MethodGet},
		prefix + api.PathPrefix + api.GetSTHConsistencyPath:  AppHandler{info: h, handler: getSTHConsistency, Name: api.GetSTHConsistencyPath, method: http.MethodGet},
		prefix + api.PathPrefix + api.GetProofByHashPath:     AppHandler{info: h, handler: getProofByHash, Name: api.GetProofByHashPath, method: http.MethodGet},
		prefix + api.PathPrefix + api.GetEntriesPath:         AppHandler{info: h, handler: getEntries, Name: api.GetEntriesPath, method: http.MethodGet},
		prefix + api.PathPrefix + api.GetSourceKeysPath:      AppHandler{info: h, handler: getSourceKeys, Name: api.GetSourceKeysPath, method: http.MethodGet},
		prefix + api.PathPrefix + api.GetLatestForSourcePath: AppHandler{info: h, handler: getLatestForSource, Name: api.GetLatestForSourcePath, method: http.MethodGet},
	}
}

// The methods below deal with HTTP and JSON encoding/decoding, but the core functionality is handled by
// the appropriate hubInfo methods.

func addSignedBlob(ctx context.Context, h *hubInfo, w http.ResponseWriter, r *http.Request) (int, error) {
	var req api.AddSignedBlobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		glog.V(1).Infof("%s: Failed to parse request body: %v", h.hubPrefix, err)
		return http.StatusBadRequest, fmt.Errorf("failed to parse add-signed-blob body: %v", err)
	}

	jsonRsp, statusCode, err := h.addSignedBlob(ctx, &req)
	if err != nil {
		return statusCode, err
	}

	w.Header().Set(contentTypeHeader, contentTypeJSON)
	jsonData, err := json.Marshal(&jsonRsp)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to JSON-marshal add-signed-blob rsp: %s", err)
	}
	if _, err = w.Write(jsonData); err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to write add-signed-blob rsp: %s", err)
	}
	glog.V(3).Infof("%s: AddSignedBlob <= SGT", h.hubPrefix)
	return http.StatusOK, nil
}

func getSTH(ctx context.Context, h *hubInfo, w http.ResponseWriter, r *http.Request) (int, error) {
	jsonRsp, statusCode, err := h.getSTH(ctx)
	if err != nil {
		return statusCode, err
	}

	jsonData, err := json.Marshal(&jsonRsp)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to marshal response: %v %v", jsonRsp, err)
	}
	w.Header().Set(contentTypeHeader, contentTypeJSON)
	if _, err := w.Write(jsonData); err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to write response data: %v", err)
	}
	return http.StatusOK, nil
}

func getSTHConsistency(ctx context.Context, h *hubInfo, w http.ResponseWriter, r *http.Request) (int, error) {
	firstVal := r.FormValue(api.GetSTHConsistencyFirst)
	if firstVal == "" {
		return http.StatusBadRequest, errors.New("parameter 'first' is required")
	}
	secondVal := r.FormValue(api.GetSTHConsistencySecond)
	if secondVal == "" {
		return http.StatusBadRequest, errors.New("parameter 'second' is required")
	}
	first, err := strconv.ParseInt(firstVal, 10, 64)
	if err != nil {
		return http.StatusBadRequest, errors.New("parameter 'first' is malformed")
	}
	second, err := strconv.ParseInt(secondVal, 10, 64)
	if err != nil {
		return http.StatusBadRequest, errors.New("parameter 'second' is malformed")
	}

	jsonRsp, statusCode, err := h.getSTHConsistency(ctx, first, second)
	if err != nil {
		return statusCode, err
	}

	w.Header().Set(contentTypeHeader, contentTypeJSON)
	jsonData, err := json.Marshal(&jsonRsp)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to marshal get-sth-consistency rsp: %v because %v", jsonRsp, err)
	}
	_, err = w.Write(jsonData)
	if err != nil {
		// Probably too late for this as headers might have been written but we don't know for sure
		return http.StatusInternalServerError, fmt.Errorf("failed to write get-sth-consistency rsp: %v because %v", jsonRsp, err)
	}
	return http.StatusOK, nil
}

func getProofByHash(ctx context.Context, h *hubInfo, w http.ResponseWriter, r *http.Request) (int, error) {
	// Accept any non empty hash that decodes from base64 and let the backend validate it further
	hash := r.FormValue(api.GetProofByHashArg)
	if len(hash) == 0 {
		return http.StatusBadRequest, errors.New("get-proof-by-hash: missing/empty hash param for get-proof-by-hash")
	}
	leafHash, err := base64.StdEncoding.DecodeString(hash)
	if err != nil {
		return http.StatusBadRequest, fmt.Errorf("get-proof-by-hash: invalid base64 hash: %v", err)
	}
	treeSize, err := strconv.ParseInt(r.FormValue(api.GetProofByHashSize), 10, 64)
	if err != nil || treeSize < 1 {
		return http.StatusBadRequest, fmt.Errorf("get-proof-by-hash: missing or invalid tree_size: %v", err)
	}

	jsonRsp, statusCode, err := h.getProofByHash(ctx, leafHash, treeSize)
	if err != nil {
		return statusCode, err
	}

	jsonData, err := json.Marshal(&jsonRsp)
	if err != nil {
		glog.Warningf("%s: Failed to marshal get-proof-by-hash rsp: %v", h.hubPrefix, jsonRsp)
		return http.StatusInternalServerError, fmt.Errorf("failed to marshal get-proof-by-hash rsp: %v, error: %v", jsonRsp, err)
	}
	w.Header().Set(contentTypeHeader, contentTypeJSON)
	if _, err := w.Write(jsonData); err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to write get-proof-by-hash rsp: %v", jsonRsp)
	}
	return http.StatusOK, nil
}

func getEntries(ctx context.Context, h *hubInfo, w http.ResponseWriter, r *http.Request) (int, error) {
	start, err := strconv.ParseInt(r.FormValue(api.GetEntriesStart), 10, 64)
	if err != nil {
		return http.StatusBadRequest, fmt.Errorf("bad start value on get-entries request: %v", err)
	}
	end, err := strconv.ParseInt(r.FormValue(api.GetEntriesEnd), 10, 64)
	if err != nil {
		return http.StatusBadRequest, fmt.Errorf("bad end value on get-entries request: %v", err)
	}

	jsonRsp, statusCode, err := h.getEntries(ctx, start, end)
	if err != nil {
		return statusCode, err
	}

	jsonData, err := json.Marshal(&jsonRsp)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to marshal get-entries rsp: %v because: %v", jsonRsp, err)
	}

	w.Header().Set(contentTypeHeader, contentTypeJSON)
	_, err = w.Write(jsonData)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to write get-entries rsp: %v because: %v", jsonRsp, err)
	}

	return http.StatusOK, nil
}

func getSourceKeys(ctx context.Context, h *hubInfo, w http.ResponseWriter, r *http.Request) (int, error) {
	jsonRsp := h.getSourceKeys(ctx)

	jsonData, err := json.Marshal(jsonRsp)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to marshal get-source-keys rsp: %v because: %v", jsonRsp, err)
	}
	w.Header().Set(contentTypeHeader, contentTypeJSON)
	if _, err := w.Write(jsonData); err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to write get-source-keys rsp: %v because: %v", jsonRsp, err)
	}
	return http.StatusOK, nil
}

func getLatestForSource(ctx context.Context, c *hubInfo, w http.ResponseWriter, r *http.Request) (int, error) {
	srcID := r.FormValue(api.GetLatestForSourceID)

	jsonRsp, statusCode, err := c.getLatestForSource(ctx, srcID)
	if err != nil {
		return statusCode, err
	}

	jsonData, err := json.Marshal(&jsonRsp)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to marshal get-latest-for-src rsp: %v because: %v", jsonRsp, err)
	}
	w.Header().Set(contentTypeHeader, contentTypeJSON)
	if _, err := w.Write(jsonData); err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to write get-latest-for-src rsp: %v because: %v", jsonRsp, err)
	}
	return http.StatusOK, nil
}

func sendHTTPError(w http.ResponseWriter, statusCode int, err error) {
	http.Error(w, fmt.Sprintf("%s\n%v", http.StatusText(statusCode), err), statusCode)
}

func checkHashSizes(path [][]byte) error {
	for i, node := range path {
		if len(node) != sha256.Size {
			return fmt.Errorf("proof[%d] is length %d, want %d", i, len(node), sha256.Size)
		}
	}
	return nil
}

func (h *hubInfo) toHTTPStatus(err error) int {
	if h.opts.ErrorMapper != nil {
		if status, ok := h.opts.ErrorMapper(err); ok {
			return status
		}
	}

	rpcStatus, ok := status.FromError(err)
	if !ok {
		return http.StatusInternalServerError
	}

	switch rpcStatus.Code() {
	case codes.OK:
		return http.StatusOK
	case codes.Canceled, codes.DeadlineExceeded:
		return http.StatusRequestTimeout
	case codes.InvalidArgument, codes.OutOfRange, codes.AlreadyExists:
		return http.StatusBadRequest
	case codes.NotFound:
		return http.StatusNotFound
	case codes.PermissionDenied, codes.ResourceExhausted:
		return http.StatusForbidden
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.FailedPrecondition:
		return http.StatusPreconditionFailed
	case codes.Aborted:
		return http.StatusConflict
	case codes.Unimplemented:
		return http.StatusNotImplemented
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
