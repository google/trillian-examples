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
	"flag"
	"net/url"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/client"
)

var (
	ftLog        = flag.String("ftlog", "http://localhost:8000", "Base URL of FT Log server")
	pollInterval = flag.Duration("poll_interval", 5*time.Second, "Duration to wait between polling for new entries")
)

func main() {
	flag.Parse()

	if len(*ftLog) == 0 {
		glog.Exit("ftlog is required")
	}

	ftURL, err := url.Parse(*ftLog)
	if err != nil {
		glog.Exitf("Failed to parse FT log URL: %q", err)
	}

	glog.Infof("Monitoring FT log %q...", *ftLog)
	ticker := time.NewTicker(*pollInterval)

	c := client.Client{LogURL: ftURL}
	var latestCP api.LogCheckpoint

	for {
		<-ticker.C
		cp, err := c.GetCheckpoint()
		if err != nil {
			glog.Warningf("Failed to update LogCheckpoint: %q", err)
			continue
		}
		if cp.TreeSize <= latestCP.TreeSize {
			continue
		}
		glog.V(1).Infof("Got newer checkpoint %s", cp)
		// TODO(al): check consistency with latestCP
		latestCP = *cp

		// TODO(al): fetch and process new entries
	}
}
