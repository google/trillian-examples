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

// Package storage provides a log storage implementation on Google Cloud Storage (GCS).
package storage

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/serverless/api"
	"github.com/google/trillian-examples/serverless/api/layout"
	"github.com/google/trillian-examples/serverless/pkg/log"
	"google.golang.org/api/iterator"

	gcs "cloud.google.com/go/storage"
)

// Client is a serverless storage implementation which uses a GCS bucket to store tree state.
// The naming of the objects of the GCS object is:
//  leaves/aa/bb/cc/ddeeff...
//  seq/aa/bb/cc/ddeeff...
//  tile/<level>/aa/bb/ccddee...
//  checkpoint
//
// The functions on this struct are not thread-safe.
type Client struct {
	gcsClient *gcs.Client
	projectID string
	// bucket is the name of the bucket where tree data will be stored.
	bucket string
	// nextSeq is a hint to the Sequence func as to what the next available
	// sequence number is to help performance.
	// Note that nextSeq may be <= than the actual next available number, but
	// never greater.
	nextSeq uint64
}

// NewClient returns a Client which allows interaction with the log implemented on GCS.
func NewClient(ctx context.Context, projectID string) (*Client, error) {
	c, err := gcs.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	return &Client{
		gcsClient: c,
		projectID: projectID,
	}, nil
}

// Create creates a new GCS bucket and returns an error on failure.
func (c *Client) Create(ctx context.Context, bucket string) error {
	bkt := c.gcsClient.Bucket(bucket)

	// If bucket has not been created, this returns error.
	if _, err := bkt.Attrs(ctx); !errors.Is(err, gcs.ErrBucketNotExist) {
		return fmt.Errorf("expected bucket %q to not be created yet (bucket attribute retrieval succeeded, expected error)",
			bucket)
	}

	if err := bkt.Create(ctx, c.projectID, nil); err != nil {
		return fmt.Errorf("failed to create bucket %q in project %s: %w", bucket, c.projectID, err)
	}
	bkt.ACL().Set(ctx, gcs.AllUsers, gcs.RoleReader)

	c.bucket = bucket
	c.nextSeq = 0
	return nil
}

// WriteCheckpoint stores a raw log checkpoint on GCS.
func (c *Client) WriteCheckpoint(ctx context.Context, newCPRaw []byte) error {
	bkt := c.gcsClient.Bucket(c.bucket)
	obj := bkt.Object(layout.CheckpointPath)
	w := obj.NewWriter(ctx)
	if _, err := w.Write(newCPRaw); err != nil {
		return err
	}
	return w.Close()
}

// ReadCheckpoint reads from GCS and returns the contents of the log checkpoint.
func (c *Client) ReadCheckpoint(ctx context.Context) ([]byte, error) {
	bkt := c.gcsClient.Bucket(c.bucket)
	obj := bkt.Object(layout.CheckpointPath)

	r, err := obj.NewReader(ctx)
	if err != nil {
			return nil, err
	}
	defer r.Close()

	return ioutil.ReadAll(r)
}

// GetTile returns the tile at the given tile-level and tile-index.
// If no complete tile exists at that location, it will attempt to find a
// partial tile for the given tree size at that location.
func (c *Client) GetTile(ctx context.Context, level, index, logSize uint64) (*api.Tile, error) {
	tileSize := layout.PartialTileSize(level, index, logSize)
	bkt := c.gcsClient.Bucket(c.bucket)

	// Pass an empty rootDir since we don't need this concept in GCS.
	objName := filepath.Join(layout.TilePath("", level, index, tileSize))
	r, err := bkt.Object(objName).NewReader(ctx)
	if err != nil {
			return nil, fmt.Errorf("failed to create reader for object %q in bucket %q: %v", objName, c.bucket, err)
	}
	defer r.Close()

	t, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read tile object %q in bucket %q: %v", objName, c.bucket, err)
	}

	var tile api.Tile
	if err := tile.UnmarshalText(t); err != nil {
		return nil, fmt.Errorf("failed to parse tile: %w", err)
	}
	return &tile, nil
}

// ScanSequenced calls the provided function once for each contiguous entry
// in storage starting at begin.
// The scan will abort if the function returns an error, otherwise it will
// return the number of sequenced entries scanned.
func (c *Client) ScanSequenced(ctx context.Context, begin uint64, f func(seq uint64, entry []byte) error) (uint64, error) {
	end := begin

	bkt := c.gcsClient.Bucket(c.bucket)
	for {
		// Pass an empty rootDir since we don't need this concept in GCS.
		sp := filepath.Join(layout.SeqPath("", end))

		// Read the object in an anonymous function so that the reader gets closed
		// in each iteration of the outside for loop.
		err := func() error {
			r, err := bkt.Object(sp).NewReader(ctx)
			if err != nil {
					return fmt.Errorf("failed to create reader for object %q in bucket %q: %v", sp, c.bucket, err)
			}
			defer r.Close()

			entry, err := ioutil.ReadAll(r)
			if errors.Is(err, gcs.ErrObjectNotExist) {
				// we're done.
				return nil
			} else if err != nil {
				return fmt.Errorf("failed to read leafdata at index %d: %w", begin, err)
			}

			if err := f(end, entry); err != nil {
				return err
			}
			end++

			return nil
		}()

		if err != nil {
			return end - begin, err
		}
	}
}

// Sequence assigns the given leaf entry to the next available sequence number.
// This method will attempt to silently squash duplicate leaves, but it cannot
// be guaranteed that no duplicate entries will exist.
// Returns the sequence number assigned to this leaf (if the leaf has already
// been sequenced it will return the original sequence number and ErrDupeLeaf).
func (c *Client) Sequence(ctx context.Context, leafhash []byte, leaf []byte) (uint64, error) {
	// 1. Check for dupe leafhash
	// 2. Write temp file
	// 3. Hard link temp -> seq file
	// 4. Create leafhash file containing assigned sequence number

	leafFQ := filepath.Join(layout.LeafPath("", leafhash))
	bkt := c.gcsClient.Bucket(c.bucket)
	r, err := bkt.Object(leafFQ).NewReader(ctx)
	if err != nil {
		return 0, err
	}
	defer r.Close()

	// Check for dupe leaf already present.
	// If there is one, it should contain the existing leaf's sequence number,
	// so read that back and return it.
	seqString, err := ioutil.ReadAll(r)
	if err != gcs.ErrObjectNotExist {
		origSeq, err := strconv.ParseUint(string(seqString), 16, 64)
		if err != nil {
			return 0, err
		}
		return origSeq, log.ErrDupeLeaf
	}

	// Now try to sequence it, we may have to scan over some newly sequenced entries
	// if Sequence has been called since the last time an Integrate/WriteCheckpoint
	// was called.
	for {
		seq := c.nextSeq

		// Write the sequence file
		seqPath := filepath.Join(layout.SeqPath("", seq))

		bkt := c.gcsClient.Bucket(c.bucket)
		wSeq := bkt.Object(seqPath).NewWriter(ctx)

		if _, err := wSeq.Write(leaf); errors.Is(err, os.ErrExist) {
			// That sequence number is in use, try the next one
			c.nextSeq++
			continue
		} else if err != nil {
			return 0, fmt.Errorf("failed to write seq file: %w", err)
		}

		// Create a leafhash file containing the assigned sequence number.
		// This isn't infallible though, if we crash after hardlinking the
		// sequence file above, but before doing this a resubmission of the
		// same leafhash would be permitted.
		leafPath := fmt.Sprintf("%s.tmp", leafFQ)
		wLeaf := bkt.Object(leafPath).NewWriter(ctx)
		if _, err := wLeaf.Write([]byte(strconv.FormatUint(seq, 16))); err != nil {
			return 0, fmt.Errorf("couldn't create leafhash object: %w", err)
		}

		// All done!
		return seq, nil
	}
}

// StoreTile writes a tile out to GCS.
// Fully populated tiles are stored at the path corresponding to the level &
// index parameters, partially populated (i.e. right-hand edge) tiles are
// stored with a .xx suffix where xx is the number of "tile leaves" in hex.
func (c *Client) StoreTile(ctx context.Context, level, index uint64, tile *api.Tile) error {
	tileSize := uint64(tile.NumLeaves)
	glog.V(2).Infof("StoreTile: level %d index %x ts: %x", level, index, tileSize)
	if tileSize == 0 || tileSize > 256 {
		return fmt.Errorf("tileSize %d must be > 0 and <= 256", tileSize)
	}
	t, err := tile.MarshalText()
	if err != nil {
		return fmt.Errorf("failed to marshal tile: %w", err)
	}

	bkt := c.gcsClient.Bucket(c.bucket)

	// Pass an empty rootDir since we don't need this concept in GCS.
	tPath := filepath.Join(layout.TilePath("", level, index, tileSize%256))
	obj := bkt.Object(tPath)

	w := obj.NewWriter(ctx)
	if _, err := w.Write(t); err != nil {
		return fmt.Errorf("failed to write tile object %q to bucket %q: %w", tPath, c.bucket, err)
	}
	return w.Close()

	if tileSize == 256 {
		// Get partial files.
		it := bkt.Objects(ctx, &gcs.Query{
			Prefix: tPath,
			// Without specifying a delimiter, the objects returned may be
			// recursively under "directories". Specifying a delimiter only returns
			// objects under the given prefix path "directory".
			Delimiter: "/",
		})
		for {
			attrs, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return fmt.Errorf("failed to get object %q from bucket %q: %v", tPath, c.bucket, err)
			}


			if _, err := bkt.Object(attrs.Name).NewWriter(ctx).Write(t); err != nil {
				return fmt.Errorf("failed to copy full tile to partials object %q in bucket %q: %v", attrs.Name, c.bucket, err)
			}
		}
	}

	return nil
}
