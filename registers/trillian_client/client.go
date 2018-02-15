package trillian_client

import (
	"context"
	"fmt"
	"log"

	"github.com/google/trillian"
	"google.golang.org/grpc"
)

const CHUNK = 10

type LogScanner interface {
	Leaf(leaf *trillian.LogLeaf) error
}

type TrillianClient interface {
	Scan(logID int64, s LogScanner) error
	Close()
}

type trillianClient struct {
	g  *grpc.ClientConn
	tc trillian.TrillianLogClient
}

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

	ts := lr.SignedLogRoot.TreeSize
	for n := int64(0); n < ts; {
		g := &trillian.GetLeavesByRangeRequest{LogId: logID, StartIndex: n, Count: CHUNK}
		r, err := t.tc.GetLeavesByRange(ctx, g)
		if err != nil {
			fmt.Errorf("Can't get leaf %d: %v", n, err)
		}

		// deal with server skew
		if r.Skew.GetTreeSizeSet() {
			ts = r.Skew.GetTreeSize()
		}

		if n < ts && len(r.Leaves) == 0 {
			fmt.Errorf("No progress at leaf %d", n)
		}

		for m := 0; m < len(r.Leaves) && n < ts; n++ {
			if r.Leaves[m] == nil {
				fmt.Errorf("Can't get leaf %d (no error)", n)
			}
			if r.Leaves[m].LeafIndex != n {
				fmt.Errorf("Got index %d expected %d", r.Leaves[n].LeafIndex, n)
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
