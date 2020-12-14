// Copyright 2020 Google LLC. All Rights Reserved.
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

// This package is the entrypoint for the Firmware Transparency monitor.
// The monitor follows the growth of the Firmware Transparency log server,
// inspects new firmware metadata as it appears, and prints out a short
// summary.
//
// TODO(al): Extend monitor to verify claims.
//
// Start the monitor using:
// go run ./cmd/ft_monitor/main.go --logtostderr -v=2 --ftlog=http://localhost:8000/
package main

import (
	"context"
	"flag"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/cmd/ft_monitor/impl"
)

var (
	ftLog        = flag.String("ftlog", "http://localhost:8000", "Base URL of FT Log server")
	pollInterval = flag.Duration("poll_interval", 5*time.Second, "Duration to wait between polling for new entries")
	keyWord      = flag.String("keyword", "trojan", "Example keyword for malware")
)

func main() {
	flag.Parse()

	if err := impl.Main(context.Background(), impl.MonitorOpts{
		LogURL:       *ftLog,
		PollInterval: *pollInterval,
		Keyword:      *keyWord,
	}); err != nil {
		glog.Exitf(err.Error())
	}
}
