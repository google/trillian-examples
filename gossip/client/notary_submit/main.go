// Copyright 2019 Google Inc. All Rights Reserved.
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

// The notary_submit binary stores a Go notary tree head in a Gossip Hub.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/golang/glog"
	"github.com/google/certificate-transparency-go/jsonclient"
	"github.com/google/trillian-examples/gossip/client"
)

var (
	skipHTTPSVerify = flag.Bool("skip_https_verify", false, "Skip verification of HTTPS transport connection")
	hubURI          = flag.String("hub_uri", "https://ct-gossip.sandbox.google.com/gamut", "Hub base URI")
	pubKey          = flag.String("pub_key", "", "Name of file containing hub's public key")

	sumURI        = flag.String("sum_uri", "https://sum.golang.org/latest", "Go module checksum database head page")
	source        = flag.String("source", "sum.golang.org", "Source name for signature")
	checkInterval = flag.Duration("check_interval", 0, "Interval between head checks; zero for one-shot")
)

func main() {
	flag.Parse()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var tlsCfg *tls.Config
	if *skipHTTPSVerify {
		glog.Warning("Skipping HTTPS connection verification")
		tlsCfg = &tls.Config{InsecureSkipVerify: *skipHTTPSVerify}
	}
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSHandshakeTimeout:   30 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			TLSClientConfig:       tlsCfg,
		},
	}
	opts := jsonclient.Options{UserAgent: "ct-go-notary-submit/1.0"}
	if *pubKey != "" {
		pubkey, err := ioutil.ReadFile(*pubKey)
		exitOnError(err)
		opts.PublicKey = string(pubkey)
	}
	glog.Infof("Use Gossip Hub at %s", *hubURI)
	client, err := client.New(*hubURI, httpClient, opts)
	exitOnError(err)

	if *checkInterval <= 0 {
		submit(ctx, *sumURI, *source, client)
		return
	}
	t := time.NewTicker(*checkInterval)
	defer t.Stop()
	for {
		submit(ctx, *sumURI, *source, client)
		select {
		case <-t.C:
		case <-ctx.Done():
			return
		}
	}
}

func submit(ctx context.Context, sumURI string, source string, hc *client.HubClient) {
	rsp, err := http.Get(sumURI)
	exitOnError(err)
	defer rsp.Body.Close()
	contents, err := ioutil.ReadAll(rsp.Body)
	exitOnError(err)
	glog.Infof("current signed note:\n%s", contents)
	sgt, err := hc.AddSignedNote(ctx, source, contents)
	exitOnError(err)
	glog.Infof("submitted to %q, got SGT %v", hc.BaseURI(), sgt)
}

func exitOnError(err error) {
	if err == nil {
		return
	}
	if err, ok := err.(jsonclient.RspError); ok {
		glog.Infof("HTTP details: status=%d, body:\n%s", err.StatusCode, err.Body)
	}
	glog.Exit(err.Error())
}
