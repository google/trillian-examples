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

func getValue(tmc trillian.TrillianMapClient, hash []byte) *string {
	index := [1][]byte{hash}
	req := &trillian.GetMapLeavesRequest{
		MapId: *mapID,
		Index: index[:],
	}

	resp, err := tmc.GetLeaves(context.Background(), req)
	if err != nil {
		log.Fatalf("Can't get leaf '%s': %v", hash, err)
	}
	if resp.MapLeafInclusion[0].Leaf.LeafValue == nil {
		return nil
	}
	s := string(resp.MapLeafInclusion[0].Leaf.LeafValue)
	return &s
}

func getRecord(tmc trillian.TrillianMapClient, k string) {
	fmt.Printf("%s\n", k)
	resp := getValue(tmc, records.RecordHash(k))
	fmt.Printf("%v\n", *resp)
}

func main() {
	flag.Parse()

	g, err := grpc.Dial(*trillianMap, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to dial Trillian Log: %v", err)
	}
	tmc := trillian.NewTrillianMapClient(g)

	if len(flag.Args()) == 0 {
		for n := 0; ; n++ {
			resp := getValue(tmc, records.KeyHash(n))
			if resp == nil {
				break
			}
			getRecord(tmc, *resp)
		}
	}

	for _, k := range flag.Args() {
		getRecord(tmc, k)
	}
}
