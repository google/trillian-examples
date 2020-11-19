// https://github.com/f-secure-foundry/armory-boot
//
// Copyright (c) F-Secure Corporation
// https://foundry.f-secure.com
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"github.com/dsoprea/go-ext4"

	"github.com/f-secure-foundry/tamago/soc/imx6/usdhc"
)

type Partition struct {
	Card    *usdhc.USDHC
	Offset  int64
	_offset int64
}

func (d *Partition) getBlockGroupDescriptor(inode int) (bgd *ext4.BlockGroupDescriptor, err error) {
	_, err = d.Seek(ext4.Superblock0Offset, io.SeekStart)

	if err != nil {
		return
	}

	sb, err := ext4.NewSuperblockWithReader(d)

	if err != nil {
		return
	}

	bgdl, err := ext4.NewBlockGroupDescriptorListWithReadSeeker(d, sb)

	if err != nil {
		return
	}

	return bgdl.GetWithAbsoluteInode(inode)
}

func (d *Partition) Read(p []byte) (n int, err error) {
	buf, err := d.Card.Read(d._offset, int64(len(p)))

	if err != nil {
		return
	}

	n = copy(p, buf)
	_, err = d.Seek(int64(n), io.SeekCurrent)

	return
}

func (d *Partition) Seek(offset int64, whence int) (int64, error) {
	info := d.Card.Info()
	end := int64(info.Blocks) * int64(info.BlockSize)

	switch whence {
	case io.SeekStart:
		d._offset = d.Offset + offset
	case io.SeekCurrent:
		d._offset += offset
	case io.SeekEnd:
		d._offset = end + d.Offset + offset
	default:
		return 0, fmt.Errorf("invalid whence %d", whence)
	}

	if d._offset > end {
		return 0, fmt.Errorf("invalid offset %d (%d)", d._offset, offset)
	}

	if d._offset < d.Offset {
		return 0, fmt.Errorf("invalid offset %d (%d)", d._offset, offset)
	}

	return d._offset, nil
}

func (d *Partition) ReadAll(fullPath string) (buf []byte, err error) {
	fullPath = strings.TrimPrefix(fullPath, "/")
	path := strings.Split(fullPath, "/")

	bgd, err := d.getBlockGroupDescriptor(ext4.InodeRootDirectory)

	if err != nil {
		return
	}

	dw, err := ext4.NewDirectoryWalk(d, bgd, ext4.InodeRootDirectory)

	var i int
	var inodeNumber int

	for {
		if err != nil {
			return
		}

		p, de, err := dw.Next()

		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		deInode := int(de.Data().Inode)

		bgd, err = d.getBlockGroupDescriptor(deInode)

		if err != nil {
			return nil, err
		}

		if p == path[i] {
			if i == len(path)-1 {
				inodeNumber = deInode
				break
			} else {
				dw, err = ext4.NewDirectoryWalk(d, bgd, deInode)

				if err != nil {
					return nil, err
				}

				i += 1
			}
		}
	}

	if inodeNumber == 0 {
		return nil, errors.New("file not found")
	}

	inode, err := ext4.NewInodeWithReadSeeker(bgd, d, inodeNumber)

	if err != nil {
		return
	}

	en := ext4.NewExtentNavigatorWithReadSeeker(d, inode)
	r := ext4.NewInodeReader(en)

	return ioutil.ReadAll(r)
}
