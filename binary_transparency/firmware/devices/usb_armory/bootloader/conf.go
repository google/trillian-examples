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

type Config struct {
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

	if len(conf.Kernel) != 2 {
		return errors.New("invalid kernel parameter size")
	}

	if len(conf.DeviceTreeBlob) != 2 {
		return errors.New("invalid kernel parameter size")
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
