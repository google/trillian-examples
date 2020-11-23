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
	"fmt"
	"log"
	"strconv"

	"github.com/f-secure-foundry/tamago/soc/imx6"
	"github.com/f-secure-foundry/tamago/soc/imx6/usdhc"

	usbarmory "github.com/f-secure-foundry/tamago/board/f-secure/usbarmory/mark-two"
)

var Build string
var Revision string

var Boot string
var StartKernel string
var StartProof string

func init() {
	log.SetFlags(0)

	if err := imx6.SetARMFreq(900); err != nil {
		panic(fmt.Sprintf("WARNING: error setting ARM frequency: %v\n", err))
	}
}

func main() {
	usbarmory.LED("blue", false)
	usbarmory.LED("white", false)

	var card *usdhc.USDHC

	switch Boot {
	case "eMMC":
		card = usbarmory.MMC
	case "uSD":
		card = usbarmory.SD
	default:
		haltAndCatchFire("invalid boot parameter", 1)
	}

	if err := card.Detect(); err != nil {
		haltAndCatchFire(fmt.Sprintf("card detect error: %v\n", err), 2)
	}

	kernelOffset, err := strconv.ParseInt(StartKernel, 10, 64)
	if err != nil {
		haltAndCatchFire(fmt.Sprintf("invalid kernel partition start offset: %v\n", err), 3)
	}
	proofOffset, err := strconv.ParseInt(StartProof, 10, 64)
	if err != nil {
		haltAndCatchFire(fmt.Sprintf("invalid proof partition start offset: %v\n", err), 3)
	}

	kernelPart := &Partition{
		Card:   card,
		Offset: kernelOffset,
	}
	if err = conf.Read(kernelPart, defaultConfigPath); err != nil {
		haltAndCatchFire(fmt.Sprintf("invalid configuration: %v\n", err), 4)
	}

	usbarmory.LED("white", true)
	h, err := hashPartition(89246720, kernelPart)
	if err != nil {
		haltAndCatchFire(fmt.Sprintf("failed to hash kernelPart: %w\n", err), 15)
	}
	fmt.Printf("Partition hash: %x", h)
	usbarmory.LED("white", false)

	proofPart := &Partition{
		Card:   card,
		Offset: proofOffset,
	}
	bundle, err := loadBundle(proofPart)
	if err != nil {
		haltAndCatchFire(fmt.Sprintf("Failed to load proof bundle: %q", err), 16)
	}
	fmt.Println("Loaded bundle: %v", bundle)



	if len(PublicKeyStr) > 0 {
		valid, err := conf.Verify(kernelPart, defaultConfigPath+signatureSuffix)

		if err != nil {
			haltAndCatchFire(fmt.Sprintf("configuration verification error: %v\n", err), 5)
		}

		if !valid {
			haltAndCatchFire("invalid configuration signature", 6)
		}
	}

	kernel, err := kernelPart.ReadAll(conf.Kernel[0])

	if err != nil {
		haltAndCatchFire(fmt.Sprintf("invalid kernel path: %v\n", err), 7)
	}

	dtb, err := kernelPart.ReadAll(conf.DeviceTreeBlob[0])

	if err != nil {
		haltAndCatchFire(fmt.Sprintf("invalid dtb path: %v\n", err), 8)
	}

	usbarmory.LED("blue", true)

	if !verifyHash(kernel, conf.Kernel[1]) {
		haltAndCatchFire("invalid kernel hash", 9)
	}

	if !verifyHash(dtb, conf.DeviceTreeBlob[1]) {
		haltAndCatchFire("invalid dtb hash", 10)
	}

	dtb, err = fixupDeviceTree(dtb, conf.CmdLine)

	if err != nil {
		haltAndCatchFire(fmt.Sprintf("dtb fixup error: %v\n", err), 11)
	}

	boot(kernel, dtb, conf.CmdLine)
}
