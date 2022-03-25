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

// Package storage provides support for accessing the SD/eMMC storage provided
// by the USB Armory.
// Note that these are very low-level primitives, and care must be taken when
// using them not to overwrite existing data (e.g. the unikernel itself!)
package storage

import "github.com/usbarmory/tamago/soc/imx6/usdhc"

var (
	// MaxTransferBytes is the largest transfer we'll attempt.
	// If we're asked to read or write more data than can fit into available DMA memeory
	// we'll had a bad time, so we'll chunk into requests of at most MaxTransferBytes bytes.
	MaxTransferBytes = 32 * 1024
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
	bs := int(d.BlockSize())
	if r := len(b) % bs; r != 0 {
		b = append(b, make([]byte, bs-r)...)
	}
	for len(b) > 0 {
		bl := len(b)
		if bl > MaxTransferBytes {
			bl = MaxTransferBytes
		}
		if err := d.Card.WriteBlocks(int(lba), b[:bl]); err != nil {
			return err
		}
		b = b[bl:]
		lba += uint(bl / bs)
	}
	return nil
}

// ReadBlocks reads data from the storage device at the given address into b.
// b must be a multiple of the underlying device's block size.
func (d *Device) ReadBlocks(lba uint, b []byte) error {
	if len(b) == 0 {
		return nil
	}
	bs := int(d.BlockSize())
	for len(b) > 0 {
		bl := len(b)
		if bl > MaxTransferBytes {
			bl = MaxTransferBytes
		}
		if err := d.Card.ReadBlocks(int(lba), b[:bl]); err != nil {
			return err
		}
		b = b[bl:]
		lba += uint(bl / bs)
	}
	return nil
}
