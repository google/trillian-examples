package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"

	"github.com/benlaurie/gds-registers/register"
	"github.com/google/trillian"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

var (
	regName     = flag.String("register", "register", "name of register (e.g. 'country')")
	trillianLog = flag.String("trillian_log", "localhost:8090", "address of the Trillian Log RPC server.")
	logID       = flag.Int64("log_id", 0, "Trillian LogID to populate.")
)

type dumper struct {
	tc               trillian.TrillianLogClient
	logID            int64
	ctx              context.Context
	newEntries       uint64
	duplicateEntries uint64
}

type leaf struct {
	Entry map[string]interface{}
	Hash  string
	Item  map[string]interface{}
}

// Process implements the register.ItemProcessor interface.
func (d *dumper) Process(e map[string]interface{}, h string, i map[string]interface{}) error {
	log.Printf("%#v %s %#v", e, h, i)

	// Put all three parts in a single structure and serialise to JSON
	l := leaf{Entry: e, Hash: h, Item: i}
	j, err := json.Marshal(l)
	if err != nil {
		return err
	}

	// Send to Trillian
	tl := &trillian.LogLeaf{LeafValue: j}
	q := &trillian.QueueLeafRequest{LogId: d.logID, Leaf: tl}
	r, err := d.tc.QueueLeaf(d.ctx, q)
	if err != nil {
		return err
	}

	// And check everything worked
	c := codes.Code(r.QueuedLeaf.GetStatus().GetCode())
	if c != codes.OK && c != codes.AlreadyExists {
		return fmt.Errorf("bad return status: %v", r.QueuedLeaf.GetStatus())
	}

	// count
	if c == codes.OK {
		d.newEntries++
	} else if c == codes.AlreadyExists {
		d.duplicateEntries++
	}
	return nil
}

func main() {
	flag.Parse()

	r, err := register.NewRegister(*regName)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%#v", r)

	g, err := grpc.Dial(*trillianLog, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to dial Trillian Log: %v", err)
	}
	defer g.Close()

	tc := trillian.NewTrillianLogClient(g)

	d := &dumper{tc: tc, ctx: context.Background(), logID: *logID}
	err = r.GetEntries(d)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("New entries: %d", d.newEntries)
	log.Printf("Duplicate entries: %d", d.duplicateEntries)
}
