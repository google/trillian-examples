// Copyright 2021 Google LLC. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package main provides a command line tool for creating signing keys
package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"os"

	"github.com/golang/glog"
	"golang.org/x/mod/sumdb/note"
)

var (
	keyName = flag.String("key_name", "", "Name for the key identity.")
	outPriv = flag.String("out_priv", "", "Output file for private key.")
	outPub  = flag.String("out_pub", "", "Output file for public key.")
	print   = flag.Bool("print", false, "Print private key, then public key, over 2 lines, to stdout.")
)

func main() {
	flag.Parse()

	if len(*keyName) == 0 {
		glog.Exit("--key_name required")
	}

	if !(*print) {
		if len(*outPriv) == 0 || len(*outPub) == 0 {
			glog.Exit("--print and/or --out_priv and --out_pub required.")
		}
	}

	skey, vkey, err := note.GenerateKey(rand.Reader, *keyName)
	if err != nil {
		glog.Exitf("Unable to create key: %q", err)
	}

	if *print {
		fmt.Println(skey)
		fmt.Println(vkey)
	}

	if len(*outPriv) > 0 && len(*outPub) > 0 {
		if err := writeFileIfNotExists(*outPriv, skey); err != nil {
			glog.Exit(err)
		}
		if err := writeFileIfNotExists(*outPub, vkey); err != nil {
			glog.Exit(err)
		}
	}
}

// Writes key files. Ensures files do not already exist to avoid accidental overwriting.
func writeFileIfNotExists(filename string, key string) error {
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("unable to create new key file %q: %w", filename, err)
	}
	_, err = file.WriteString(key)
	if err != nil {
		return fmt.Errorf("unable to write new key file %q: %w", filename, err)
	}
	file.Close()
	return nil
}
