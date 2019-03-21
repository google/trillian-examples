// Package trillian_client provides some useful utilities for
// interacting with Trillian.
package trillian_client

import (
	"context"
	"fmt"
	"log"

	"github.com/google/trillian"
	"github.com/google/trillian/types"
	"google.golang.org/grpc"
)

const chunk = 10

// A type that is passed to TrillianClient.Scan(). Leaf() is called on
// it for each leaf in the log.
type LogScanner interface {
	Leaf(leaf *trillian.LogLeaf) error
}

// A Trillian client. Create a new one with trillian_client.New().
type TrillianClient interface {
	Scan(logID int64, s LogScanner) error
	Close()
}

type trillianClient struct {
	g  *grpc.ClientConn
	tc trillian.TrillianLogClient
}

// New creates and connects new TrillianClient, given the URL of the
// Trillian server.
func New(logAddr string) TrillianClient {
	g, err := grpc.Dial(logAddr, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to dial Trillian Log: %v", err)
	}

	tc := trillian.NewTrillianLogClient(g)

	return &trillianClient{g, tc}
}

func (t *trillianClient) Scan(logID int64, s LogScanner) error {
	ctx := context.Background()

	rr := &trillian.GetLatestSignedLogRootRequest{LogId: logID}
	lr, err := t.tc.GetLatestSignedLogRoot(ctx, rr)
	if err != nil {
		log.Fatalf("Can't get log root: %v", err)
	}

	var root types.LogRootV1
	// TODO(Martin2112): Verify root signature.
	if err := root.UnmarshalBinary(lr.SignedLogRoot.LogRoot); err != nil {
		return fmt.Errorf("Root failed to unmarshal: %v", err)
	}

	ts := root.TreeSize
	for n := uint64(0); n < ts; {
		g := &trillian.GetLeavesByRangeRequest{LogId: logID, StartIndex: int64(n), Count: chunk}
		r, err := t.tc.GetLeavesByRange(ctx, g)
		if err != nil {
			return fmt.Errorf("Can't get leaf %d: %v", n, err)
		}

		// Deal with server skew, if tree size has reduced.
		// Don't allow increases so this terminates eventually.
		rts := root.TreeSize
		if rts < ts {
			ts = rts
		}

		if n < ts && len(r.Leaves) == 0 {
			return fmt.Errorf("No progress at leaf %d", n)
		}

		for m := 0; m < len(r.Leaves) && n < ts; n++ {
			if r.Leaves[m] == nil {
				return fmt.Errorf("Can't get leaf %d (no error)", n)
			}
			if uint64(r.Leaves[m].LeafIndex) != n {
				return fmt.Errorf("Got index %d expected %d", r.Leaves[n].LeafIndex, n)
			}
			err := s.Leaf(r.Leaves[m])
			if err != nil {
				return err
			}
			m++
		}
	}
	return nil
}

func (t *trillianClient) Close() {
	t.g.Close()
}
