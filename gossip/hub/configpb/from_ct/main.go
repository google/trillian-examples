// Copyright 2018 Google Inc. All Rights Reserved.
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

// from_ct is a utility for generating "source" config stanzas from a
// Certificate Transparency log list JSON file.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/certificate-transparency-go/loglist"
	"github.com/google/certificate-transparency-go/x509util"
	"github.com/google/trillian-examples/gossip/hub/configpb"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/crypto/sigpb"
)

var (
	logList = flag.String("log_list", loglist.LogListURL, "Location of master log list (URL or filename)")
)

func main() {
	flag.Parse()
	client := &http.Client{Timeout: time.Second * 10}

	llData, err := x509util.ReadFileOrURL(*logList, client)
	if err != nil {
		glog.Exitf("Failed to read log list: %v", err)
	}

	ll, err := loglist.NewFromJSON(llData)
	if err != nil {
		glog.Exitf("Failed to build log list: %v", err)
	}
	for _, l := range ll.Logs {
		url := l.URL
		if !strings.HasPrefix(url, "https://") {
			url = "https://" + url
		}
		tlog := configpb.TrackedSource{
			Name:          l.Description,
			Id:            url,
			PublicKey:     &keyspb.PublicKey{Der: l.Key},
			HashAlgorithm: sigpb.DigitallySigned_SHA256, // RFC6962 s2.1 mandates SHA-256
		}
		fmt.Printf("source {\n")
		proto.MarshalText(os.Stdout, &tlog)
		fmt.Printf("}\n")
	}
}
