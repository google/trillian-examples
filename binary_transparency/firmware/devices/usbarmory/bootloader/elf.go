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
	"bytes"
	"debug/elf"
	"fmt"

	"github.com/usbarmory/tamago/dma"
)

// loadELF attempts to load the provided ELF image.
//
// This function implements a _very_ simple ELF loader which is suitable for
// loading bare-metal ELF files like those produced by TamaGo.
func loadELF(mem uint, kernel []byte) (addr uint) {
	f, err := elf.NewFile(bytes.NewReader(kernel))

	if err != nil {
		panic(err)
	}

	for idx, prg := range f.Progs {
		if prg.Type != elf.PT_LOAD {
			continue
		}

		b := make([]byte, prg.Memsz)

		_, err := prg.ReadAt(b[0:prg.Filesz], 0)

		if err != nil {
			panic(fmt.Sprintf("failed to read LOAD section at idx %d, %q", idx, err))
		}

		offset := uint(prg.Paddr) - mem
		dma.Write(mem, int(offset), b)
	}

	return uint(f.Entry)
}
