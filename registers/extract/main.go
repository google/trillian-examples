package main

import (
	"context"
	"flag"
	"log"

	"github.com/google/trillian"
	"google.golang.org/grpc"
)

var (
	trillianLog = flag.String("trillian_log", "localhost:8090", "address of the Trillian Log RPC server.")
	logID       = flag.Int64("log_id", 0, "Trillian LogID to populate.")
)

const CHUNK = 10

func main() {
	flag.Parse()

	g, err := grpc.Dial(*trillianLog, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to dial Trillian Log: %v", err)
	}
	defer g.Close()

	tc := trillian.NewTrillianLogClient(g)
	ctx := context.Background()

	rr := &trillian.GetLatestSignedLogRootRequest{LogId: *logID}
	lr, err := tc.GetLatestSignedLogRoot(ctx, rr)
	if err != nil {
		log.Fatalf("Can't get log root: %v", err)
	}

	ts := lr.SignedLogRoot.TreeSize
	for n := int64(0); n < ts; {
		g := &trillian.GetLeavesByRangeRequest{LogId: *logID, StartIndex: n, Count: CHUNK}
		r, err := tc.GetLeavesByRange(ctx, g)
		if err != nil {
			log.Fatalf("Can't get leaf %d: %v", n, err)
		}

		// deal with server skew
		if r.Skew.GetTreeSizeSet() {
			ts = r.Skew.GetTreeSize()
		}

		for m := 0; m < CHUNK && n < ts; n++ {
			if r.Leaves[m] == nil {
				log.Fatalf("Can't get leaf %d (no error)", n)
			}
			log.Printf("leaf %d: %v", n, r.Leaves[m])
			m++
		}
	}
}
