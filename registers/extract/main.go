package main

import (
	"flag"
	"log"

	"github.com/google/trillian"
	"github.com/google/trillian-examples/registers/trillian_client"
)

var (
	trillianLog = flag.String("trillian_log", "localhost:8090", "address of the Trillian Log RPC server.")
	logID       = flag.Int64("log_id", 0, "Trillian LogID to populate.")
)

type logScanner struct {
}

func (*logScanner) Leaf(n int64, leaf *trillian.LogLeaf) error {
	log.Printf("leaf %d: %v", n, leaf)
	return nil
}

func main() {
	flag.Parse()

	tc := trillian_client.New(*trillianLog)
	defer tc.Close()

	err := tc.Scan(*logID, &logScanner{})
	if err != nil {
		log.Fatal(err)
	}
}
