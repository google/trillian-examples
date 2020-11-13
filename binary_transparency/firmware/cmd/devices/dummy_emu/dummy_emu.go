// dummy_emu is an "emulator" for the dummy device.
//
// This emulator shows how the firmware transparency artifacts stored
// when firmware is updates are validated on boot to protect the device against
// tampering.
//
// The device's initialisation routines in "ROM" are where this validation
// takes place, and the device only "jumps into" the firmware payload once
// successful validation has taken place.
//
// Usage:
//   go run ./cmd/devices/dummy_emu --logtostderr --dummy_storage_dir=/tmp/dummy_device
package main

import (
	"flag"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/binary_transparency/firmware/cmd/devices/dummy_emu/rom"
)

func main() {
	flag.Parse()

	boot, err := rom.ResetFromFlags()
	if err != nil {
		glog.Exitf("ROM: %q", err)
	}

	if err := boot(); err != nil {
		glog.Warningf("boot(): %q", err)
	}
}
