// Copyright 2020 Google LLC
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

// Package audit contains all the components needed to clone the SumDB into a
// local database and verify that the data downloaded matches that guaranteed
// by the Checkpoint advertised by the SumDB service. It also provides support
// for parsing the downloaded data and verifying that no module+version is
// every provided with different checksums.
package audit

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	backoff "github.com/cenkalti/backoff/v4"
	"github.com/golang/glog"
	"github.com/google/trillian/merkle/compact"
	"golang.org/x/mod/sumdb/tlog"
	"golang.org/x/sync/errgroup"
)

// CheckpointTooFresh is returned when a consistency proof is requested for a
// checkpoint that is larger than the currently synced size.
type CheckpointTooFresh struct {
	RequestedSize, MaxSize int64
}

func (e CheckpointTooFresh) Error() string {
	return fmt.Sprintf("requested checkpoint is at size %d but local log only has %d entries", e.RequestedSize, e.MaxSize)
}

// InconsistentCheckpoints is returned when a consistency proof cannot be found
// between two checkpoints.
type InconsistentCheckpoints struct {
	A, B *Checkpoint
}

func (e InconsistentCheckpoints) Error() string {
	return fmt.Sprintf("checkpoints at size %d (%x) and size %d (%x) are inconsistent", e.A.N, e.A.Hash, e.B.N, e.B.Hash)
}

// Service has all the operations required for an auditor to verifiably clone
// the remote SumDB.
type Service struct {
	localDB *Database
	sumDB   *SumDBClient
	rf      *compact.RangeFactory
	height  int
}

// NewService constructs a new Service which is ready to go.
func NewService(localDB *Database, sumDB *SumDBClient, height int) *Service {
	rf := &compact.RangeFactory{
		Hash: func(left, right []byte) []byte {
			var lHash, rHash tlog.Hash
			copy(lHash[:], left)
			copy(rHash[:], right)
			thash := tlog.NodeHash(lHash, rHash)
			return thash[:]
		},
	}
	return &Service{
		localDB: localDB,
		sumDB:   sumDB,
		rf:      rf,
		height:  height,
	}
}

// GoldenCheckpoint gets the previously checked Checkpoint, or nil if not found.
func (s *Service) GoldenCheckpoint(ctx context.Context) *Checkpoint {
	golden, err := s.localDB.GoldenCheckpoint(s.sumDB.ParseCheckpointNote)
	if err != nil {
		glog.Infof("failed to find previously stored golden checkpoint: %v", err)
		return nil
	}
	return golden
}

// Sync brings the local sqlite database up to date with the latest full tiles published
// in the remote SumDB and committed to by the provided checkpoint. It checks that the
// previous golden checkpoint is consistent with the new data, and that the checkpoint
// passed in commits to the new data which was fetched.
// This method is not atomic. If any of the verification checks fail then the database
// will be updated with whatever was fetched. This means that there will be an audit
// trail, but it could mean that the local DB needs to be started from scratch if the
// remote SumDB is repaired.
func (s *Service) Sync(ctx context.Context, checkpoint *Checkpoint) error {
	if err := s.cloneLeafTiles(ctx, checkpoint); err != nil {
		return fmt.Errorf("cloneLeafTiles: %w", err)
	}
	glog.V(1).Infof("Updated leaves to latest checkpoint (tree size %d). Calculating hashes...", checkpoint.N)

	if err := s.hashTiles(ctx, checkpoint); err != nil {
		return fmt.Errorf("hashTiles: %w", err)
	}
	glog.V(1).Infof("Hashes updated successfully. Checking consistency with previous checkpoint...")
	if err := s.checkConsistency(ctx); err != nil {
		return fmt.Errorf("checkConsistency: %w", err)
	}
	glog.V(1).Infof("Log consistent. Checking root hash with remote...")
	if err := s.checkRootHash(ctx, checkpoint); err != nil {
		return fmt.Errorf("checkRootHash: %w", err)
	}
	glog.V(1).Infof("Log sync and verification complete")
	return s.localDB.SetGoldenCheckpoint(checkpoint)
}

// ProcessMetadata parses the leaf data and writes the semantic data into the DB.
func (s *Service) ProcessMetadata(ctx context.Context, checkpoint *Checkpoint) error {
	tileWidth := 1 << s.height
	metadata := make([]Metadata, tileWidth)

	latest, err := s.localDB.MaxLeafMetadata(ctx)
	var firstTile int64
	if err != nil {
		switch err.(type) {
		case NoDataFound:
			glog.Infof("failed to find head of Leaf Metadata, assuming empty and starting from scratch: %v", err)
			firstTile = 0
		default:
			return fmt.Errorf("MaxLeafMetadata(): %w", err)
		}
	} else {
		firstTile = (latest + 1) / int64(tileWidth)
	}

	for offset := firstTile; offset < checkpoint.N/int64(tileWidth); offset++ {
		leafOffset := int64(offset) * int64(tileWidth)
		hashes, err := s.localDB.Leaves(leafOffset, tileWidth)
		if err != nil {
			return err
		}
		for i, h := range hashes {
			leafID := leafOffset + int64(i)

			lines := strings.Split(string(h), "\n")
			tokens := strings.Split(lines[0], " ")
			module, version, repoHash := tokens[0], tokens[1], tokens[2]
			tokens = strings.Split(lines[1], " ")
			if got, want := tokens[0], module; got != want {
				return fmt.Errorf("mismatched module names at %d: (%s, %s)", leafID, got, want)
			}
			if got, want := tokens[1][:len(version)], version; got != want {
				return fmt.Errorf("mismatched version names at %d: (%s, %s)", leafID, got, want)
			}
			modHash := tokens[2]

			metadata[i] = Metadata{module, version, repoHash, modHash}
		}
		if err := s.localDB.SetLeafMetadata(ctx, leafOffset, metadata); err != nil {
			return err
		}
	}
	return nil
}

// CheckConsistency checks that the given checkpoint is consistent with the state
// downloaded and committed to by the current Golden Checkpoint. If the given
// checkpoint is too recent then a CheckpointTooFresh error will be returned. If
// the checkpoint is inconsistent with the golden checkpoint then the error will
// be of type InconsistentCheckpoints.
func (s *Service) CheckConsistency(ctx context.Context, checkpoint *Checkpoint) error {
	golden, err := s.localDB.GoldenCheckpoint(s.ParseCheckpointNote)
	if err != nil {
		return fmt.Errorf("failed to query for golden checkpoint: %w", err)
	}
	if bytes.Equal(checkpoint.Raw, golden.Raw) {
		// This is easy and possibly common, so handle it as a special case.
		return nil
	}
	head, err := s.localDB.Head()
	if err != nil {
		return fmt.Errorf("cannot find head of local log: %w", err)
	}
	if checkpoint.N > head {
		// If we wanted to, we _could_ check consistency for checkpoints that
		// are larger than this, but it would require querying the log. For the
		// sake of a tight design, we will only try to perform checks for data
		// which we have entirely locally.
		return CheckpointTooFresh{
			RequestedSize: checkpoint.N,
			MaxSize:       head}
	}

	err = s.checkCheckpoint(checkpoint, func(stragglerCount int) ([][]byte, error) {
		stragglerTileOffset := int(checkpoint.N / (1 << s.height))
		return s.localDB.Tile(s.height, 0, stragglerTileOffset)
	})
	if err != nil {
		return InconsistentCheckpoints{
			A: golden,
			B: checkpoint,
		}
	}
	return nil
}

// ParseCheckpointNote parses the raw checkpoint.
func (s *Service) ParseCheckpointNote(raw []byte) (*Checkpoint, error) {
	return s.sumDB.ParseCheckpointNote(raw)
}

// VerifyTiles checks that every tile calculated locally matches the result returned
// by SumDB. This shouldn't be necessary if checkRootHash is working, but this may be
// useful to determine where any corruption has happened in the tree.
func (s *Service) VerifyTiles(ctx context.Context, checkpoint *Checkpoint) error {
	for level := 0; level <= s.getLevelsForLeafCount(checkpoint.N); level++ {
		finishedLevel := false
		offset := 0
		for !finishedLevel {
			localHashes, err := s.localDB.Tile(s.height, level, offset)
			if err != nil {
				if err == sql.ErrNoRows {
					finishedLevel = true
					continue
				}
				return fmt.Errorf("failed to get tile hashes: %w", err)
			}
			sumDBHashes, err := s.sumDB.TileHashes(level, offset)
			if err != nil {
				return fmt.Errorf("failed to get tile hashes: %w", err)
			}

			for i := 0; i < 1<<s.height; i++ {
				var lHash tlog.Hash
				copy(lHash[:], localHashes[i])
				if sumDBHashes[i] != lHash {
					return fmt.Errorf("found mismatched hash at L=%d, O=%d, leaf=%d\n\tlocal : %x\n\tremote: %x", level, offset, i, sumDBHashes[i][:], localHashes[i])
				}
			}
			offset++
		}
	}
	return nil
}

// cloneLeafTiles copies the leaf data from the SumDB into the local database.
// It only copies whole tiles, which means that some stragglers may not be
// copied locally.
func (s *Service) cloneLeafTiles(ctx context.Context, checkpoint *Checkpoint) error {
	head, err := s.localDB.Head()
	if err != nil {
		switch err.(type) {
		case NoDataFound:
			glog.Infof("failed to find head of database, assuming empty and starting from scratch: %v", err)
			head = -1
		default:
			return fmt.Errorf("failed to query for head of local log: %w", err)
		}
	}
	if checkpoint.N < head {
		return fmt.Errorf("illegal state; more leaves locally (%d) than in SumDB (%d)", head, checkpoint.N)
	}
	localLeaves := head + 1

	tileWidth := int64(1 << s.height)
	remainingLeaves := checkpoint.N - localLeaves
	remainingChunks := int(remainingLeaves / tileWidth)
	startOffset := int(localLeaves / tileWidth)

	if remainingChunks > 0 {
		glog.Infof("fetching %d new tiles starting at offset %d", remainingChunks, startOffset)

		leafChan := make(chan tileLeaves)
		errChan := make(chan error)

		// Start a goroutine to sequentially fetch each of the full tiles that the local
		// mirror is missing, using exponential backoff on each fetch in case of failure.
		// Successfully fetched tiles are passed back using leafChan.
		// Errors are passed back using errChan.
		go func() {
			for i := 0; i < remainingChunks; i++ {
				var c tileLeaves
				operation := func() error {
					offset := startOffset + i
					leaves, err := s.sumDB.FullLeavesAtOffset(offset)
					if err != nil {
						return err
					}
					c = tileLeaves{int64(offset) * tileWidth, leaves}
					return nil
				}
				err := backoff.Retry(operation, backoff.NewExponentialBackOff())
				if err != nil {
					errChan <- err
				} else {
					leafChan <- c
				}
			}
		}()

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		// Consume the output from the goroutine above, writing tiles to the local DB.
		for i := 0; i < remainingChunks; i++ {
			// Status updates if verbosity is high enough.
			if glog.V(2) {
				select {
				case <-ticker.C:
					glog.V(2).Infof("cloning tile %d of %d", i, remainingChunks)
				default:
				}
			}

			select {
			case err := <-errChan:
				return err
			case chunk := <-leafChan:
				start, leaves := chunk.start, chunk.data
				err = s.localDB.WriteLeaves(ctx, start, leaves)
				if err != nil {
					return fmt.Errorf("WriteLeaves: %w", err)
				}
			}
		}
	}
	return nil
}

// hashTiles performs a full recalculation of all the tiles using the data from
// the leaves table. Any hashes that no longer match what was previously stored
// will cause an error. Any new hashes will be filled in.
// The results of the hashing are stored in the local DB.
// This could be replaced by something more incremental if the performance is
// unnacceptable. While the SumDB is still reasonably small, this is fine as is.
func (s *Service) hashTiles(ctx context.Context, checkpoint *Checkpoint) error {
	tileWidth := 1 << s.height
	tileCount := int(checkpoint.N / int64(tileWidth))

	g := new(errgroup.Group)
	roots := make(chan *compact.Range, tileWidth)

	leafTileCount := tileCount
	leafRoots := roots
	glog.V(1).Infof("hashing %d leaf tiles", leafTileCount)
	g.Go(func() error { return s.hashLeafLevel(leafTileCount, leafRoots) })

	for i := 1; i <= s.getLevelsForLeafCount(checkpoint.N); i++ {
		tileCount /= tileWidth

		thisLevel := i
		thisTileCount := tileCount
		in := roots

		outRoots := make(chan *compact.Range, tileWidth)

		glog.V(1).Infof("hashing %d tiles at level %d", thisTileCount, thisLevel)
		g.Go(func() error { return s.hashUpperLevel(thisLevel, thisTileCount, in, outRoots) })

		roots = outRoots
	}
	if err := g.Wait(); err != nil {
		return fmt.Errorf("failed to hash: %w", err)
	}
	return nil
}

// checkConsistency checks that the downloaded and hashed tiles are consistent with
// the previously fetched Golden Checkpoint. Any previously fetched tiles are checked
// by hashTiles, so this is only really checking that any stragglers transiently acquired
// when verifying the Golden Checkpoint made it into the completed tile unchanged.
func (s *Service) checkConsistency(ctx context.Context) error {
	golden, err := s.localDB.GoldenCheckpoint(s.sumDB.ParseCheckpointNote)
	if err != nil {
		switch err.(type) {
		case NoDataFound:
			// TODO(mhutchinson): This should fail hard later. Making tolerant for now
			// so that previous databases can be cheaply upgraded (golden checkpoint
			// storage is a new feature).
			glog.Warning("Failed to find golden checkpoint!")
			return nil
		default:
			return fmt.Errorf("failed to query for golden checkpoint: %w", err)
		}
	}
	head, err := s.localDB.Head()
	if err != nil {
		return fmt.Errorf("cannot find head of local log: %w", err)
	}
	if golden.N > head {
		// This can happen if SumDB hasn't committed to any new complete tiles since the
		// last run of the auditor. Any changes in tile hashes will be caught by hashTiles
		// so skipping the consistency check is safer than it appears.
		glog.Infof("golden STH is at size %d but local log only has %d entries; consistency check skipped", golden.N, head)
		return nil
	}

	err = s.checkCheckpoint(golden, func(stragglerCount int) ([][]byte, error) {
		stragglerTileOffset := int(golden.N / (1 << s.height))
		return s.localDB.Tile(s.height, 0, stragglerTileOffset)
	})
	if err != nil {
		return fmt.Errorf("local log failed consistency with previous golden checkpoint: %w", err)
	}
	return nil
}

// checkRootHash calculates the root hash from the locally generated tiles, and then
// appends any stragglers from the SumDB, returning an error if this calculation
// fails or the result does not match that in the checkpoint provided.
func (s *Service) checkRootHash(ctx context.Context, checkpoint *Checkpoint) error {
	err := s.checkCheckpoint(checkpoint, func(stragglerCount int) ([][]byte, error) {
		stragglerTileOffset := int(checkpoint.N / (1 << s.height))
		stragglers, err := s.sumDB.PartialLeavesAtOffset(stragglerTileOffset, stragglerCount)
		if err != nil {
			return nil, fmt.Errorf("failed to get stragglers: %w", err)
		}
		res := make([][]byte, stragglerCount)
		for i, s := range stragglers {
			sHash := tlog.RecordHash(s)
			res[i] = sHash[:]
		}
		return res, nil
	})
	if err != nil {
		return fmt.Errorf("local log does not match the SumDB checkpoint: %w", err)
	}
	return nil
}

// checkCheckpoint ensures that the local log matches the commitment in the given checkpoint.
// Complete tiles will be read from local storage, and any stragglers from the final incomplete
// tile will be requested via the function.
func (s *Service) checkCheckpoint(cp *Checkpoint, getStragglers func(stragglerCount int) ([][]byte, error)) error {
	logRange := s.rf.NewEmptyRange(0)

	// Calculate the whole log hash starting from left to right. We could compute these
	// hashes just using the leaf level tiles all the way across, but it's more efficient
	// to use the highest level tile possible. We start at the highest level, and then
	// descend down a level each time we run out of completed tiles at that level. By the
	// end we will be down to the leaf level and we will have computed the hashes for all
	// completed tiles.
	for level := s.getLevelsForLeafCount(cp.N); level >= 0; level-- {
		// how many real leaves a tile at this level covers.
		tileLeafCount := uint64(1) << ((level + 1) * s.height)
		levelTileCount := int(uint64(cp.N) / tileLeafCount)
		firstTileOffset := int(logRange.End() / tileLeafCount)

		for offset := firstTileOffset; offset < levelTileCount; offset++ {
			tHashes, err := s.localDB.Tile(s.height, level, offset)
			if err != nil {
				return fmt.Errorf("failed to get tile L=%d, O=%d: %v", level, offset, err)
			}
			// Calculate this tile as a standalone subtree
			tcr := s.rf.NewEmptyRange(0)
			for _, t := range tHashes {
				tcr.Append(t, nil)
			}
			// Now use the range as what it really is; a commitment to a larger number of leaves
			treeRange, err := s.rf.NewRange(uint64(offset)*tileLeafCount, uint64(offset+1)*tileLeafCount, tcr.Hashes())
			if err != nil {
				return fmt.Errorf("failed to create range for tile L=%d, O=%d: %v", level, offset, err)
			}
			// Append this into the running log range.
			logRange.AppendRange(treeRange, nil)
		}
	}

	// Now add all of the stragglers to the logRange.
	stragglersCount := int(uint64(cp.N) - logRange.End())
	if stragglersCount > 0 {
		stragglers, err := getStragglers(stragglersCount)
		if err != nil {
			return fmt.Errorf("failed to get stragglers: %w", err)
		}
		for i := 0; i < stragglersCount; i++ {
			logRange.Append(stragglers[i], nil)
		}
	}

	// Now check the logRange matches the checkpoint.
	if logRange.End() != uint64(cp.N) {
		return fmt.Errorf("calculation error, found %d leaves but was aiming for %d", logRange.End(), cp.N)
	}
	root, err := logRange.GetRootHash(nil)
	if err != nil {
		return fmt.Errorf("failed to get root hash: %w", err)
	}
	var rootHash tlog.Hash
	copy(rootHash[:], root)
	if rootHash != cp.Hash {
		return fmt.Errorf("log root mismatch at tree size %d; calculated %x, want %x", cp.N, root, cp.Hash[:])
	}
	return nil
}

func (s *Service) hashLeafLevel(tileCount int, roots chan<- *compact.Range) error {
	for offset := 0; offset < tileCount; offset++ {
		hashes, err := s.localDB.Tile(s.height, 0, offset)
		if err == sql.ErrNoRows {
			hashes, err = s.hashLeafTile(offset)
		}
		if err != nil {
			return err
		}
		cr := s.rf.NewEmptyRange(uint64(offset) * 1 << s.height)
		for _, h := range hashes {
			cr.Append(h, nil)
		}
		if got, want := len(cr.Hashes()), 1; got != want {
			return fmt.Errorf("expected single root hash but got %d", got)
		}
		roots <- cr
	}
	return nil
}

func (s *Service) hashLeafTile(offset int) ([][]byte, error) {
	tileWidth := 1 << s.height

	leaves, err := s.localDB.Leaves(int64(tileWidth)*int64(offset), tileWidth)
	if err != nil {
		return nil, fmt.Errorf("failed to get leaves from DB: %w", err)
	}
	res := make([][]byte, tileWidth)
	leafHashes := make([]byte, tileWidth*HashLenBytes)
	for i, l := range leaves {
		recordHash := tlog.RecordHash(l)
		res[i] = recordHash[:]
		copy(leafHashes[i*HashLenBytes:], res[i])
	}
	return res, s.localDB.SetTile(s.height, 0, offset, leafHashes)
}

func (s *Service) hashUpperLevel(level, tileCount int, in <-chan *compact.Range, out chan<- *compact.Range) error {
	tileWidth := 1 << s.height

	inHashes := make([][]byte, tileWidth)
	tileHashBlob := make([]byte, tileWidth*HashLenBytes)
	for offset := 0; offset < tileCount; offset++ {
		dbTileHashes, err := s.localDB.Tile(s.height, level, offset)
		found := true
		if err == sql.ErrNoRows {
			found = false
			err = nil
		}
		if err != nil {
			return err
		}
		for i := 0; i < tileWidth; i++ {
			cr := <-in
			inHashes[i] = cr.Hashes()[0]
			copy(tileHashBlob[i*HashLenBytes:], inHashes[i])

			if found && !bytes.Equal(dbTileHashes[i], inHashes[i]) {
				return fmt.Errorf("got diffence in hash at L=%d, O=%d, leaf=%d", level, offset, i)
			}
		}

		if !found {
			if err := s.localDB.SetTile(s.height, level, offset, tileHashBlob); err != nil {
				return fmt.Errorf("failed to set tile at L=%d, O=%d: %v", level, offset, err)
			}
		}
		cr := s.rf.NewEmptyRange(uint64(offset * tileWidth))
		for _, h := range inHashes {
			cr.Append(h, nil)
		}
		if got, want := len(cr.Hashes()), 1; got != want {
			return fmt.Errorf("expected single root hash but got %d", got)
		}
		out <- cr
	}
	return nil
}

// getLevelsForLeafCount determines how many strata of tiles of the configured
// height are needed to contain the largest perfect subtree that can be made of
// the leaves.
func (s *Service) getLevelsForLeafCount(leaves int64) int {
	topLevel := -1
	coveredIdx := leaves >> s.height
	for coveredIdx > 0 {
		coveredIdx = coveredIdx >> s.height
		topLevel++
	}
	return topLevel
}

// tileLeaves is a contiguous block of leaves within a tile.
type tileLeaves struct {
	start int64    // The leaf index of the first leaf
	data  [][]byte // The leaf data
}
