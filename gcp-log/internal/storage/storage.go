// Package storage provides a log storage implementation on Google Cloud Storage (GCS).
package storage

import (
	"context"
	"fmt"
	"path/filepath"

	gcs "cloud.google.com/go/storage"
	"github.com/google/trillian-examples/serverless/api/layout"
)

// Client is a serverless storage implementation which uses a GCS bucket to store tree state.
// The naming of the objects of the GCS object is:
//  <rootDir>/leaves/aa/bb/cc/ddeeff...
//  <rootDir>/leaves/pending/aabbccddeeff...
//  <rootDir>/seq/aa/bb/cc/ddeeff...
//  <rootDir>/tile/<level>/aa/bb/ccddee...
//  <rootDir>/checkpoint
//
// The functions on this struct are not thread-safe.
type Client struct {
	gcsClient *gcs.Client
	projectID string
	// rootDir is the root directory where tree data will be stored.
	rootDir string
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
// TODO(jayhou): is empty string for rootDir acceptable?
func (c *Client) Create(ctx context.Context, rootDir string) error {
	bkt := c.gcsClient.Bucket(rootDir)
	// If bucket has not been created, this returns error.
	if _, err := bkt.Attrs(ctx); err == nil {
		return fmt.Errorf("expected bucket '%s' to not be created yet (bucket attribute retrieval succeeded, expected error)",
			rootDir)
	}

	// TODO(jayhou): specify bktAttrs to allow read access (0755).
	if err := bkt.Create(ctx, c.projectID, nil); err != nil {
		return fmt.Errorf("failed to create bucket %q in project %s: %w", rootDir, c.projectID, err)
	}

	c.rootDir = rootDir
	c.nextSeq = 0
	return nil
}

// WriteCheckpoint stores a raw log checkpoint on GCS.
func (c *Client) WriteCheckpoint(ctx context.Context, newCPRaw []byte) error {
	bkt := c.gcsClient.Bucket(c.rootDir)
	obj := bkt.Object(filepath.Join(c.rootDir, layout.CheckpointPath))
	w := obj.NewWriter(ctx)
	if _, err := w.Write(newCPRaw); err != nil {
		return err
	}
	return w.Close()
}
