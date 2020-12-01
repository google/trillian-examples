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

	usbarmory "github.com/f-secure-foundry/tamago/board/f-secure/usbarmory/mark-two"
	"github.com/f-secure-foundry/tamago/dma"
	"github.com/f-secure-foundry/tamago/soc/imx6"
)

var Build string
var Revision string

var Boot string
var StartKernel string
var StartProof string

var PublicKeyStr string

var (
	partition      *Partition
	proofPartition *Partition
)

func init() {
	usbarmory.LED("blue", false)
	usbarmory.LED("white", false)

	log.SetFlags(0)

	if err := imx6.SetARMFreq(900); err != nil {
		panic(fmt.Sprintf("cannot change ARM frequency, %v\n", err))
	}

	kernOffset, err := strconv.ParseInt(StartKernel, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("invalid start kernel offset, %v\n", err))
	}
	partition = &Partition{
		Offset: kernOffset,
	}

	proofOffset, err := strconv.ParseInt(StartProof, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("invalid proof partition start offset: %v\n", err))
	}
	proofPartition = &Partition{
		Offset: proofOffset,
	}

	switch Boot {
	case "eMMC":
		partition.Card = usbarmory.MMC
		proofPartition.Card = usbarmory.MMC
	case "uSD":
		partition.Card = usbarmory.SD
		proofPartition.Card = usbarmory.SD
	default:
		panic("invalid boot parameter")
	}
}

func main() {
	if err := partition.Card.Detect(); err != nil {
		panic(fmt.Sprintf("boot media error, %v\n", err))
	}

	usbarmory.LED("blue", true)

	if err := conf.Init(partition, defaultConfigPath); err != nil {
		panic(fmt.Sprintf("configuration error, %v\n", err))
	}

	h, err := hashPartition(partition)
	if err != nil {
		panic(fmt.Sprintf("failed to hash kernelPart: %w\n", err))
	}
	fmt.Printf("partition hash: 0x%x\n", h)

	bundle, err := loadBundle(proofPartition)
	if err != nil {
		panic(fmt.Sprintf("Failed to load proof bundle: %q", err))
	}
	fmt.Printf("loaded bundle: %v\n", bundle)

	if len(PublicKeyStr) > 0 {
		err := conf.Verify(defaultConfigPath+signatureSuffix, PublicKeyStr)

		if err != nil {
			panic(fmt.Sprintf("configuration error, %v\n", err))
		}
	} else {
		log.Printf("armory-boot: no public key, skipping signature verification")
	}

	err = conf.Load()

	if err != nil {
		panic(fmt.Sprintf("configuration error, %v\n", err))
	}

	if !verifyHash(conf.kernel, conf.kernelHash) {
		panic("invaid kernel hash")
	}

	if len(conf.params) > 0 {
		if !verifyHash(conf.params, conf.paramsHash) {
			panic("invalid dtb hash")
		}

		conf.params, err = fixupDeviceTree(conf.params, conf.CmdLine)

		if err != nil {
			panic(fmt.Sprintf("dtb fixup error, %v\n", err))
		}
	}

	usbarmory.LED("white", true)

	dma.Init(dmaStart, dmaSize)
	mem, _ := dma.Reserve(dmaSize, 0)

	if conf.elf {
		boot(loadELF(mem, conf.kernel), 0)
	} else {
		dma.Write(mem, conf.kernel, kernelOffset)
		dma.Write(mem, conf.params, paramsOffset)

		boot(mem+kernelOffset, mem+paramsOffset)
	}
}
