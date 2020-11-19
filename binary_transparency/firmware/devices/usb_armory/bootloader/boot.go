// https://github.com/f-secure-foundry/armory-boot
//
// Copyright (c) F-Secure Corporation
// https://foundry.f-secure.com
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/f-secure-foundry/tamago/arm"
	"github.com/f-secure-foundry/tamago/board/f-secure/usbarmory/mark-two"
	"github.com/f-secure-foundry/tamago/dma"
	"github.com/f-secure-foundry/tamago/soc/imx6"
)

// defined in boot.s
func exec(kernel uint32, params uint32)
func svc()

func boot(kernel []byte, dtb []byte, cmdline string) {
	dma.Init(dmaStart, dmaSize)
	mem, _ := dma.Reserve(dmaSize, 0)

	dma.Write(mem, kernel, kernelOffset)
	dma.Write(mem, dtb, dtbOffset)

	image := mem + kernelOffset
	params := mem + dtbOffset

	arm.ExceptionHandler(func(n int) {
		if n != arm.SUPERVISOR {
			panic("unhandled exception")
		}

		fmt.Printf("armory-boot: starting kernel image@%x params@%x\n", image, params)

		usbarmory.LED("blue", false)
		usbarmory.LED("white", false)

		// Linux RNGB driver doesn't play well with a previously
		// initialized RNGB, therefore reset it.
		imx6.RNGB.Reset()

		imx6.ARM.InterruptsDisable()
		imx6.ARM.CacheFlushData()
		imx6.ARM.CacheDisable()

		exec(image, params)
	})

	svc()
}
