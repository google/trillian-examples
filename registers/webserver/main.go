package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
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

// FIXME: rather than use a global, make a closure around serveRecords
var tmc trillian.TrillianMapClient

func fixRecord(j string) ([]byte, string) {
	var v map[string]interface{}
	json.Unmarshal([]byte(j), &v)
	log.Printf("%v", v)

	f := make(map[string]interface{})
	i := v["Entry"].(map[string]interface{})
	for _, s := range []string{"entry-number", "entry-timestamp", "index-entry-number", "key"} {
		f[s] = i[s]
	}
	f["item"] = v["Items"]

	jj, err := json.Marshal(f)
	if err != nil {
		log.Fatal(err)
	}

	return jj, f["key"].(string)
}

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

	io.WriteString(w, "{")
	for n := start; n < start+size; n++ {
		resp := records.GetValue(tmc, *mapID, records.KeyHash(n))
		if resp == nil {
			break
		}
		resp = records.GetValue(tmc, *mapID, records.RecordHash(*resp))
		// FIXME: not formatted exactly like GDS registers...
		//fmt.Fprintf(w, "%s\n", *resp)
		r, k := fixRecord(*resp)
		if n != start {
			io.WriteString(w, ",")
		}
		fmt.Fprintf(w, "\"%s\":%s", k, r)
	}
	io.WriteString(w, "}")
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
