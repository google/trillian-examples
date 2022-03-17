// https://github.com/usbarmory/armory-boot
//
// Copyright (c) F-Secure Corporation
// https://foundry.f-secure.com
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

//go:build armory
// +build armory

package main

import (
	"log"

	"github.com/usbarmory/tamago/arm"
	usbarmory "github.com/usbarmory/tamago/board/f-secure/usbarmory/mark-two"
	"github.com/usbarmory/tamago/soc/imx6"
	"github.com/usbarmory/tamago/soc/imx6/rngb"
)

// defined in boot.s
func exec(kernel uint32, params uint32)
func svc()

func boot(kernel uint32, params uint32) {
	arm.SystemExceptionHandler = func(n int) {
		if n != arm.SUPERVISOR {
			panic("unhandled exception")
		}

		log.Printf("armory-boot: starting kernel@%x params@%x\n", kernel, params)

		usbarmory.LED("blue", false)
		usbarmory.LED("white", false)

		// RNGB driver doesn't play well with previous initializations
		rngb.Reset()

		imx6.ARM.DisableInterrupts()
		imx6.ARM.FlushDataCache()
		imx6.ARM.DisableCache()

		exec(kernel, params)
	}

	svc()
}
