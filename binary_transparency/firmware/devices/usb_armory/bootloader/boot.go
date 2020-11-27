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
	"bytes"
	"debug/elf"
	"fmt"

	"github.com/f-secure-foundry/tamago/arm"
	"github.com/f-secure-foundry/tamago/board/f-secure/usbarmory/mark-two"
	"github.com/f-secure-foundry/tamago/dma"
	"github.com/f-secure-foundry/tamago/soc/imx6"
	"github.com/f-secure-foundry/tamago/soc/imx6/rngb"
)

// defined in boot.s
func exec(kernel uint32, params uint32)
func svc()

func bootKernel(kernel []byte, dtb []byte, cmdline string) {
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
		rngb.Reset()

		imx6.ARM.InterruptsDisable()
		imx6.ARM.CacheFlushData()
		imx6.ARM.CacheDisable()

		exec(image, params)
	})

	svc()
}

// bootELFUnikernel attempts to load the provided ELF image and jumps to it.
//
// This function implements a _very_ simple ELF loader which is suitable for
// loading bare-metal ELF files like those produced by TamaGo.
func bootELFUnikernel(img []byte) {
	dma.Init(dmaStart, dmaSize)
	mem, _ := dma.Reserve(dmaSize, 0)

	f, err := elf.NewFile(bytes.NewReader(img))

	if err != nil {
		panic(err.Error)
	}

	for idx, prg := range f.Progs {
		if prg.Type != elf.PT_LOAD {
			continue
		}

		b := make([]byte, prg.Memsz)

		_, err := prg.ReadAt(b[0:prg.Filesz], 0)

		if err != nil {
			panic(fmt.Sprintf("armory-boot: failed to read LOAD section at idx %d: %q", idx, err))
		}

		offset := uint32(prg.Paddr) - mem
		dma.Write(mem, b, int(offset))
	}

	entry := f.Entry

	arm.ExceptionHandler(func(n int) {
		if n != arm.SUPERVISOR {
			panic("unhandled exception")
		}

		fmt.Printf("armory-boot: starting unikernel image@%x\n", entry)

		usbarmory.LED("blue", false)
		usbarmory.LED("white", false)

		rngb.Reset()
		imx6.ARM.InterruptsDisable()
		imx6.ARM.CacheFlushData()
		imx6.ARM.CacheDisable()

		// We can re-use the kernel exec even though we don't really need
		// some of the functionality in there.
		exec(uint32(entry), 0)
	})

	svc()
}
