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

// Package testonly provides support for storage tests.
package testonly

import "testing"

const MemBlockSize = 512

//  MemDev is a simple in-memory block device.
type MemDev [][MemBlockSize]byte

func (md MemDev) BlockSize() uint {
	return MemBlockSize
}

func (md MemDev) ReadBlocks(lba uint, b []byte) error {
	lenB := uint(len(b))
	if lba > lenB {
		return nil
	}
	bl := lenB / MemBlockSize
	if l := uint(len(md)); lba+bl > l {
		bl = l - lba
	}
	for i := uint(0); i < bl; i++ {
		copy(b[i*MemBlockSize:], md[lba+i][:])
	}
	return nil
}

func (md MemDev) WriteBlocks(lba uint, b []byte) error {
	lenB := uint(len(b))
	if lba > lenB {
		return nil
	}
	bl := lenB / MemBlockSize
	if l := uint(len(md)); lba+bl > l {
		bl = l - lba
	}
	for i := uint(0); i < bl; i++ {
		copy(md[lba+i][:], b[i*MemBlockSize:])
	}
	return nil
}

// NewMemDev creates a new in-memory block device.
func NewMemDev(t *testing.T, numBlocks uint) MemDev {
	t.Helper()
	return make(MemDev, numBlocks)
}
