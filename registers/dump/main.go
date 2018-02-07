package main

import (
	"flag"
	"log"

	"github.com/benlaurie/gds-registers/register"
)

var (
	regName = flag.String("register", "register", "name of register (e.g. 'country')")
)

type dumper struct {
}

// Process implements the register.ItemProcessor interface.
func (*dumper) Process(e map[string]interface{}, h string, i map[string]interface{}) error {
	log.Printf("%#v %s %#v", e, h, i)
	return nil
}

func main() {
	flag.Parse()

	r, err := register.NewRegister(*regName)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%#v", r)

	err = r.GetEntries(&dumper{})
	if err != nil {
		log.Fatal(err)
	}
}
