// https://github.com/f-secure-foundry/armory-boot
//
// Copyright (c) F-Secure Corporation
// https://foundry.f-secure.com
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

//+build armory

package main

import (
	"log"

	"github.com/f-secure-foundry/tamago/arm"
	"github.com/f-secure-foundry/tamago/board/f-secure/usbarmory/mark-two"
	"github.com/f-secure-foundry/tamago/soc/imx6"
	"github.com/f-secure-foundry/tamago/soc/imx6/rngb"
)

// defined in boot.s
func exec(kernel uint32, params uint32)
func svc()

func boot(kernel uint32, params uint32) {
	arm.ExceptionHandler(func(n int) {
		if n != arm.SUPERVISOR {
			panic("unhandled exception")
		}

		log.Printf("armory-boot: starting kernel@%x params@%x\n", kernel, params)

		usbarmory.LED("blue", false)
		usbarmory.LED("white", false)

		// RNGB driver doesn't play well with previous initializations
		rngb.Reset()

		imx6.ARM.InterruptsDisable()
		imx6.ARM.CacheFlushData()
		imx6.ARM.CacheDisable()

		exec(kernel, params)
	})

	svc()
}
