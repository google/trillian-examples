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
var Start string

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

	usbarmory.LED("white", true)

	offset, err := strconv.ParseInt(Start, 10, 64)
	if err != nil {
		haltAndCatchFire(fmt.Sprintf("invalid start offset: %v\n", err), 3)
	}

	partition := &Partition{
		Card:   card,
		Offset: offset,
	}

	if err = conf.Read(partition, defaultConfigPath); err != nil {
		haltAndCatchFire(fmt.Sprintf("invalid configuration: %v\n", err), 4)
	}

	if len(PublicKeyStr) > 0 {
		valid, err := conf.Verify(partition, defaultConfigPath+signatureSuffix)

		if err != nil {
			haltAndCatchFire(fmt.Sprintf("configuration verification error: %v\n", err), 5)
		}

		if !valid {
			haltAndCatchFire("invalid configuration signature", 6)
		}
	}

	kernel, err := partition.ReadAll(conf.Kernel[0])

	if err != nil {
		haltAndCatchFire(fmt.Sprintf("invalid kernel path: %v\n", err), 7)
	}

	dtb, err := partition.ReadAll(conf.DeviceTreeBlob[0])

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
