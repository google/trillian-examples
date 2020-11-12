// publish is a demo tool to put firmware metadata into the log.
package main

import (
	"context"
	"crypto/sha512"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	"github.com/google/trillian-examples/binary_transparency/firmware/internal/client"
)

var (
	logURL = flag.String("log_url", "http://localhost:8000", "Base URL of the log HTTP API")

	deviceID   = flag.String("device", "TalkieToaster", "the target device for the firmware")
	revision   = flag.Uint64("revision", 1, "the version of the firmware")
	binaryPath = flag.String("binary_path", "", "file path to the firmware binary")
	timestamp  = flag.String("timestamp", "", "timestamp formatted as RFC3339, or empty to use current time")
	timeout    = flag.Duration("timeout", 5*time.Minute, "Duration to wait for inclusion of submitted metadata")
)

func main() {
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	logURL, err := url.Parse(*logURL)
	if err != nil {
		glog.Exitf("logURL is invalid: %v", err)
	}

	metadata, err := createManifestFromFlags()
	if err != nil {
		glog.Exitf("Failed to create manifest: %v", err)
	}

	js, err := createStatementJSON(metadata)
	if err != nil {
		glog.Exitf("Failed to marshal statement: %v", err)
	}

	c := &client.Client{
		LogURL: logURL,
	}

	initialCP, err := c.GetCheckpoint()
	if err != nil {
		glog.Exitf("Failed to get a pre-submission checkpoint from log: %q", err)
	}

	glog.Info("Submitting entry...")
	if err := c.SubmitManifest(js); err != nil {
		glog.Exitf("Couldn't submit statement: %v", err)
	}

	glog.Info("Successfully submitted entry, waiting for inclusion...")
	cp, consistency, ip, err := client.AwaitInclusion(ctx, c, *initialCP, js)
	if err != nil {
		glog.Errorf("Failed while waiting for inclusion: %v", err)
		glog.Warningf("Failed checkpoint: %s", cp)
		glog.Warningf("Failed consistency proof: %x", consistency)
		glog.Warningf("Failed inclusion proof: %x", ip)
		glog.Exit("Bailing.")
	}

	glog.Infof("Successfully logged %s", js)
}

func createManifestFromFlags() (api.FirmwareMetadata, error) {
	file, err := os.Open(*binaryPath)
	if err != nil {
		return api.FirmwareMetadata{}, fmt.Errorf("failed to open '%s' for reading: %w", *binaryPath, err)
	}
	defer file.Close()

	h := sha512.New()
	if _, err := io.Copy(h, file); err != nil {
		return api.FirmwareMetadata{}, fmt.Errorf("failed to read '%s': %w", *binaryPath, err)
	}

	buildTime := *timestamp
	if buildTime == "" {
		buildTime = time.Now().Format(time.RFC3339)
	}
	metadata := api.FirmwareMetadata{
		DeviceID:                    *deviceID,
		FirmwareRevision:            *revision,
		FirmwareImageSHA512:         h.Sum(nil),
		ExpectedFirmwareMeasurement: []byte{}, // TODO: This should be provided somehow.
		BuildTimestamp:              buildTime,
	}

	return metadata, nil
}

func createStatementJSON(m api.FirmwareMetadata) ([]byte, error) {
	js, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	statement := api.FirmwareStatement{
		Metadata:  js,
		Signature: []byte("LOL!"),
	}

	return json.Marshal(statement)
}
