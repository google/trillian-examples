// publish is a demo tool to put firmware metadata into the log.
package main

import (
	"bytes"
	"crypto/sha512"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
)

var (
	logAddress = flag.String("log_address", "localhost:8000", "server:port of the log HTTP API")

	deviceID   = flag.String("device", "TalkieToaster", "the target device for the firmware")
	revision   = flag.Uint64("revision", 1, "the version of the firmware")
	binaryPath = flag.String("binary_path", "", "file path to the firmware binary")
	timestamp  = flag.String("timestamp", "", "timestamp formatted as RFC3339, or empty to use current time")
)

func main() {
	flag.Parse()

	file, err := os.Open(*binaryPath)
	if err != nil {
		glog.Exitf("failed to open '%s' for reading: %v", *binaryPath, err)
	}
	defer file.Close()

	h := sha512.New()
	if _, err := io.Copy(h, file); err != nil {
		glog.Exitf("failed to read '%s': %v", *binaryPath, err)
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
	js, err := json.Marshal(metadata)
	if err != nil {
		glog.Exitf("failed to marshal metadata: %v", err)
	}

	statement := api.FirmwareStatement{
		Metadata:  js,
		Signature: []byte("LOL!"),
	}

	js, err = json.Marshal(statement)
	if err != nil {
		glog.Exitf("failed to marshal statement: %v", err)
	}

	httpPath := fmt.Sprintf("http://%s/%s", *logAddress, api.HTTPAddFirmware)
	resp, err := http.Post(httpPath, "application/json", bytes.NewBuffer(js))
	if err != nil {
		glog.Exitf("failed to publish to log endpoint (%s): %v", httpPath, err)
	}
	if resp.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			glog.Exitf("failed to parse body of response! %v", err)
		}
		glog.Exitf("failed to publish to log: %s\n%s", resp.Status, string(body))
	}
	glog.Infof("Successfully logged %s", statement)
}
