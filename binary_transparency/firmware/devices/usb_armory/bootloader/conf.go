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
	"encoding/json"
	"errors"
	"fmt"
	"log"
)

const defaultConfigPath = "/boot/armory-boot.conf"

var conf Config

// Config tells the boot-loader what it should attempt to load and boot.
//
// The bootloader can load either a Linux kernel, or an ELF unikernel.
type Config struct {
	// Unikernel is the path to an ELF unikernel image.
	Unikernel []string `json:"unikernel"`

	// Kernel is the path to a Linux kernel image.
	Kernel         []string `json:"kernel"`
	DeviceTreeBlob []string `json:"dtb"`
	CmdLine        string   `json:"cmdline"`

	conf []byte
}

func (c *Config) Read(partition *Partition, configPath string) (err error) {
	log.Printf("armory-boot: reading configuration at %s\n", configPath)

	c.conf, err = partition.ReadAll(configPath)

	if err != nil {
		return
	}

	err = json.Unmarshal(c.conf, &c)

	if err != nil {
		return
	}

	ul, kl := len(conf.Unikernel), len(conf.Kernel)
	isUnikernel, isKernel := ul > 0, kl > 0

	if isUnikernel == isKernel {
		return errors.New("must specify either unikernel or kernel")
	}

	switch {
	case isKernel:
		if kl != 2 {
			return errors.New("invalid kernel parameter size")
		}

		if len(conf.DeviceTreeBlob) != 2 {
			return errors.New("invalid dtb parameter size")
		}
	case isUnikernel:
		if ul != 2 {
			return errors.New("invalid unikernel parameter size")
		}
	default:
		panic("armory-boot: config is neither for kernel nor unikernel")
	}

	c.Print()

	return
}

func (c *Config) Verify(partition *Partition, sigPath string) (valid bool, err error) {
	log.Println("armory-boot: verifying configuration signature")

	sig, err := partition.ReadAll(sigPath)

	if err != nil {
		return false, fmt.Errorf("invalid signature path, %v", err)
	}

	return verifySignature(c.conf, sig)
}

func (c *Config) Print() {
	j, _ := json.MarshalIndent(c, "", "\t")
	log.Printf("\n%s", string(j))
}
