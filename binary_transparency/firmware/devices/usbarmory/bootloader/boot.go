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
	usbarmory "github.com/usbarmory/tamago/board/usbarmory/mk2"
	"github.com/usbarmory/tamago/soc/nxp/imx6ul"
)

// defined in boot.s
func exec(kernel uint32, params uint32)
func svc()

func boot(kernel uint, params uint) {
	arm.SystemExceptionHandler = func(n int) {
		if n != arm.SUPERVISOR {
			panic("unhandled exception")
		}

		log.Printf("armory-boot: starting kernel@%x params@%x\n", kernel, params)

		usbarmory.LED("blue", false)
		usbarmory.LED("white", false)

		// RNGB driver doesn't play well with previous initializations
		imx6ul.RNGB.Reset()

		imx6ul.ARM.FlushDataCache()
		imx6ul.ARM.DisableCache()

		exec(uint32(kernel), uint32(params))
	}

	svc()
}
