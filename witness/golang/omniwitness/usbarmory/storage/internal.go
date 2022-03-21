// Copyright 2022 Google LLC. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build usbarmory
// +build usbarmory

// Package storage provides support for using the SD/eMMC storage provided
// by the USB Armory.
package storage

import "github.com/usbarmory/tamago/soc/imx6/usdhc"

var (
	// MaxTransfer is the largest transfer we'll attempt.
	// If we're asked to read or write more data than can fit into available DMA memeory
	// we'll had a bad time, so we'll chunk into requests of at most MaxTransfer bytes.
	MaxTransfer = 32 * 1024
)

// Device allows writing to one of the USB Armory storage peripherals, hiding some
// of the sharp edges around DMA etc.
type Device struct {
	Card *usdhc.USDHC
}

// BlockSize returns the size in bytes of the each block in the underlying storage.
func (d *Device) BlockSize() uint {
	return uint(d.Card.Info().BlockSize)
}

// WriteBlocks writes the data in b to the device blocks starting at the given block address.
// If the final block to be written is partial, it will be padded with zeroes to ensure that
// full blocks are written.
func (d *Device) WriteBlocks(lba uint, b []byte) error {
	if len(b) == 0 {
		return nil
	}
	ci := d.Card.Info()
	// If the requested write is not a multiple of the block size, tack on
	// some zeros to make it be.
	if r := len(b) % ci.BlockSize; r != 0 {
		b = append(b, make([]byte, ci.BlockSize-r)...)
	}
	for len(b) > 0 {
		bl := len(b)
		if bl > MaxTransfer {
			bl = MaxTransfer
		}
		if err := d.Card.WriteBlocks(int(lba), b[:bl]); err != nil {
			return err
		}
		b = b[bl:]
		lba += uint(bl / ci.BlockSize)
	}
	return nil
}

// ReadBlocks reads data from the storage device at the given address into b.
// b must be a multiple of the underlying device's block size.
func (d *Device) ReadBlocks(lba uint, b []byte) error {
	if len(b) == 0 {
		return nil
	}
	ci := d.Card.Info()
	for len(b) > 0 {
		bl := len(b)
		if bl > MaxTransfer {
			bl = MaxTransfer
		}
		if err := d.Card.ReadBlocks(int(lba), b[:bl]); err != nil {
			return err
		}
		b = b[bl:]
		lba += uint(bl / ci.BlockSize)
	}
	return nil
}
