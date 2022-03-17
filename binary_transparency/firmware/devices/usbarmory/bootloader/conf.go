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
	"encoding/json"
	"errors"
	"fmt"
	"log"
)

const defaultConfigPath = "/boot/armory-boot.conf"
const signatureSuffix = ".sig"

var conf Config

type Config struct {
	// Kernel is the path to a Linux kernel image.
	Kernel []string `json:"kernel"`

	// DeviceTreeBlob is the path to a Linux DTB file.
	DeviceTreeBlob []string `json:"dtb"`

	// CmdLine is the Linux kernel command-line parameters.
	CmdLine string `json:"cmdline"`

	// Unikernel is the path to an ELF unikernel image.
	Unikernel []string `json:"unikernel"`

	partition *Partition
	conf      []byte

	kernel     []byte
	kernelHash string

	params     []byte
	paramsHash string

	elf bool
}

func (c *Config) Init(p *Partition, configPath string) (err error) {
	log.Printf("armory-boot: reading configuration at %s\n", configPath)

	c.partition = p
	c.conf, err = p.ReadAll(configPath)

	return
}

func (c *Config) Verify(sigPath string, pubKey string) (err error) {
	sig, err := c.partition.ReadAll(sigPath)

	if err != nil {
		return fmt.Errorf("invalid signature path, %v", err)
	}

	return verifySignature(c.conf, sig, pubKey)
}

func (c *Config) Load() (err error) {
	err = json.Unmarshal(c.conf, &c)

	if err != nil {
		return
	}

	ul, kl := len(conf.Unikernel), len(conf.Kernel)
	isUnikernel, isKernel := ul > 0, kl > 0

	if isUnikernel == isKernel {
		return errors.New("must specify either unikernel or kernel")
	}

	var kernelPath string

	switch {
	case isKernel:
		if kl != 2 {
			return errors.New("invalid kernel parameter size")
		}

		if len(conf.DeviceTreeBlob) != 2 {
			return errors.New("invalid dtb parameter size")
		}

		kernelPath = conf.Kernel[0]
		c.kernelHash = conf.Kernel[1]
	case isUnikernel:
		if ul != 2 {
			return errors.New("invalid unikernel parameter size")
		}

		kernelPath = conf.Unikernel[0]
		c.kernelHash = conf.Unikernel[1]
	}

	c.Print()

	c.kernel, err = c.partition.ReadAll(kernelPath)

	if err != nil {
		return fmt.Errorf("invalid path %s, %v\n", kernelPath, err)
	}

	if isUnikernel {
		c.elf = true
		return
	}

	c.paramsHash = conf.DeviceTreeBlob[1]
	c.params, err = c.partition.ReadAll(conf.DeviceTreeBlob[0])

	if err != nil {
		return fmt.Errorf("invalid path %s, %v\n", conf.DeviceTreeBlob[0], err)
	}

	return
}

func (c *Config) Print() {
	j, _ := json.MarshalIndent(c, "", "\t")
	log.Printf("\n%s", string(j))
}
