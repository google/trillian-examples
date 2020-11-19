// https://github.com/f-secure-foundry/armory-boot
//
// Copyright (c) F-Secure Corporation
// https://foundry.f-secure.com
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

package main

import (
	"bytes"

	"github.com/u-root/u-root/pkg/dt"
)

func fixupDeviceTree(dtb []byte, cmdline string) (dtbFixed []byte, err error) {
	fdt, err := dt.ReadFDT(bytes.NewReader(dtb))

	if err != nil {
		return
	}

	for _, node := range fdt.RootNode.Children {
		if node.Name == "chosen" {
			bootargs := dt.Property{
				Name:  "bootargs",
				Value: []byte(cmdline + "\x00"),
			}

			node.Properties = append(node.Properties, bootargs)
		}

		// temporary fixup until newer dtbs with correct device_type
		// information are released
		if node.Name == "memory" {
			device_type := dt.Property{
				Name:  "device_type",
				Value: []byte("memory" + "\x00"),
			}

			node.Properties = append(node.Properties, device_type)
		}
	}

	dtbBuf := new(bytes.Buffer)
	_, err = fdt.Write(dtbBuf)

	if err != nil {
		panic(err)
	}

	return dtbBuf.Bytes(), nil
}
