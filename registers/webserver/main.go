package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/google/trillian"
	"github.com/google/trillian-examples/registers/records"
	"google.golang.org/grpc"
)

var (
	trillianMap = flag.String("trillian_map", "localhost:8095", "address of the Trillian Map RPC server.")
	mapID       = flag.Int64("map_id", 0, "Trillian MapID to read.")
)

var tmc trillian.TrillianMapClient

func serveRecords(w http.ResponseWriter, r *http.Request) {
	var err error

	r.ParseForm()

	s := r.Form["page-size"]
	size := 100
	if s != nil {
		size, err = strconv.Atoi(s[0])
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	i := r.Form["page-index"]
	start := 0
	if i != nil {
		start, err = strconv.Atoi(i[0])
		if err != nil || start < 1 {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Sigh. Start index is 1? Really?
		start = (start - 1) * size
	}

	for n := start; n < start+size; n++ {
		resp := records.GetValue(tmc, *mapID, records.KeyHash(n))
		if resp == nil {
			break
		}
		resp = records.GetValue(tmc, *mapID, records.RecordHash(*resp))
		// FIXME: not formatted exactly like GDS registers...
		fmt.Fprintf(w, "%s\n", *resp)
	}
}

func main() {
	flag.Parse()

	g, err := grpc.Dial(*trillianMap, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to dial Trillian Log: %v", err)
	}
	tmc = trillian.NewTrillianMapClient(g)

	http.HandleFunc("/records.json", serveRecords)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
