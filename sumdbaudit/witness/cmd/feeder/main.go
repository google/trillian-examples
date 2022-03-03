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

// feeder polls the sumdb log and pushes the results to a generic witness.
package main

import (
	"context"
	"flag"
	"net/http"
	"net/url"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/formats/log"
	"github.com/google/trillian-examples/internal/feeder/sumdb"
	"github.com/google/trillian-examples/serverless/config"
	wclient "github.com/google/trillian-examples/witness/golang/client/http"
	"golang.org/x/mod/sumdb/note"
)

var (
	vkey         = flag.String("k", "sum.golang.org+033de0ae+Ac4zctda0e5eza+HJyk9SxEdh+s3Ux18htTTAD8OuAn8", "key")
	origin       = flag.String("origin", "go.sum database tree", "The expected first origin log (first line of the checkpoint)")
	witness      = flag.String("w", "", "The endpoint of the witness HTTP REST API")
	witnessKey   = flag.String("wk", "", "The public key of the witness")
	logID        = flag.String("lid", "", "The ID of the log within the witness")
	pollInterval = flag.Duration("poll", 10*time.Second, "How quickly to poll the sumdb to get updates")
)

func main() {
	flag.Parse()
	ctx := context.Background()
	wURL, err := url.Parse(*witness)
	if err != nil {
		glog.Exitf("Failed to parse witness URL: %v", err)
	}

	w := wclient.Witness{
		URL:      wURL,
		Verifier: mustCreateVerifier(*witnessKey),
	}

	lid := *logID
	if len(lid) == 0 {
		lid = log.ID(*origin, []byte(*vkey))
	}

	log := config.Log{
		Origin:    *origin,
		PublicKey: *vkey,
		ID:        lid,
	}
	if err := sumdb.FeedLog(ctx, log, w, http.DefaultClient, *pollInterval); err != nil {
		glog.Exitf("Feeder: %v", err)
	}
}

func mustCreateVerifier(pub string) note.Verifier {
	v, err := note.NewVerifier(pub)
	if err != nil {
		glog.Exitf("Failed to create signature verifier from %q: %v", pub, err)
	}
	return v
}
