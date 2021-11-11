// https://github.com/f-secure-foundry/armory-boot
//
// Copyright (c) F-Secure Corporation
// https://foundry.f-secure.com
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

//go:build console && armory
// +build console,armory

package main

import (
	"fmt"
	"log"
	"runtime"
	"time"

	usbarmory "github.com/f-secure-foundry/tamago/board/f-secure/usbarmory/mark-two"
	"github.com/f-secure-foundry/tamago/soc/imx6"
)

func init() {
	debugConsole, _ := usbarmory.DetectDebugAccessory(250 * time.Millisecond)
	<-debugConsole

	banner := fmt.Sprintf("armory-boot • %s/%s (%s) • %s %s • %s",
		runtime.GOOS, runtime.GOARCH, runtime.Version(),
		Revision, Build,
		imx6.Model())

	log.SetFlags(0)
	log.Printf("%s", banner)
}
