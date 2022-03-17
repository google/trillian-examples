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
	_ "unsafe"
)

// Override imx6ul.ramStart, usbarmory.ramSize and dma allocation, as the
// mapped kernel image needs to be within the first 128MiB of RAM.

//go:linkname ramStart runtime.ramStart
var ramStart uint32 = 0x90000000

//go:linkname ramSize runtime.ramSize
var ramSize uint32 = 0x10000000

var dmaStart uint32 = 0x80000000
var dmaSize = 0x10000000

const kernelOffset = 0x0800000
const paramsOffset = 0x7000000
