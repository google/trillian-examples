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

// Package main provides a command line tool for managing signing keys
package main

import (
	"crypto/rand"
	"flag"
	"fmt"

	"github.com/golang/glog"
	"golang.org/x/mod/sumdb/note"
)

var keyName = flag.String("key_name", "", "Name for the key identity")

func main() {
	flag.Parse()

	if len(*keyName) == 0 {
		glog.Exit("--key_name required")
	}

	skey, _, err := note.GenerateKey(rand.Reader, *keyName)
	if err != nil {
		glog.Exitf("Unable to create key: %q", err)
	}
	fmt.Println(skey)
}
