package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/google/trillian"
	"github.com/google/trillian-examples/registers/records"
	"google.golang.org/grpc"
)

var (
	trillianMap = flag.String("trillian_map", "localhost:8095", "address of the Trillian Map RPC server.")
	mapID       = flag.Int64("map_id", 0, "Trillian MapID to read.")
)

func main() {
	flag.Parse()

	g, err := grpc.Dial(*trillianMap, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to dial Trillian Log: %v", err)
	}
	tmc := trillian.NewTrillianMapClient(g)

	for _, k := range flag.Args() {
		fmt.Printf("%s\n", k)
		hash := records.RecordHash(k)
		index := [1][]byte{hash}
		req := &trillian.GetMapLeavesRequest{
			MapId: *mapID,
			Index: index[:],
		}

		resp, err := tmc.GetLeaves(context.Background(), req)
		if err != nil {
			log.Fatalf("Can't get leaf '%s': %v", k, err)
		}
		fmt.Printf("%v\n", string(resp.MapLeafInclusion[0].Leaf.LeafValue))
	}
}
