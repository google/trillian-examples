// Copyright 2021 Google LLC. All Rights Reserved.
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

// Package layout contains routines for specifying the on-disk layout of the
// stored log.
// This will be used by both the storage package, as well as clients accessing
// the stored data either directly or via some other transport.
package layout

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// StatePath is the location of the file containing the log checkpoint.
	StatePath = "state"
)

// SeqPath builds the directory path and relative filename for the entry at the given
// sequence number.
func SeqPath(root string, seq uint64) (string, string) {
	frag := []string{
		root,
		"seq",
		fmt.Sprintf("%02x", (seq >> 32)),
		fmt.Sprintf("%02x", (seq>>24)&0xff),
		fmt.Sprintf("%02x", (seq>>16)&0xff),
		fmt.Sprintf("%02x", (seq>>8)&0xff),
		fmt.Sprintf("%02x", seq&0xff),
	}
	d := filepath.Join(frag[:6]...)
	return d, frag[6]
}

// SeqFromPath recovers a sequence number from the specified path.
// The path must have been generated with the SeqPath method in this package.
func SeqFromPath(root, seqPath string) (uint64, error) {
	seqDir := filepath.Join(root, "seq")
	if !strings.HasPrefix(seqPath, seqDir) {
		return 0, fmt.Errorf("invalid seqPath %q, doesn't start with prefix %q", seqPath, seqDir)
	}
	s := strings.TrimPrefix(seqPath, seqDir)
	if s[0] != os.PathSeparator || s[3] != os.PathSeparator || s[6] != os.PathSeparator || s[9] != os.PathSeparator || s[12] != os.PathSeparator {
		return 0, fmt.Errorf("seqPath format invalid %q", s)
	}
	var b strings.Builder
	for _, s := range []string{s[1:3], s[4:6], s[7:9], s[10:12], s[13:]} {
		b.WriteString(s)
	}
	return strconv.ParseUint(b.String(), 16, 64)
}

// LeafPath builds the directory path and relative filename for the entry data with the
// given leafhash.
func LeafPath(root string, leafhash []byte) (string, string) {
	frag := []string{
		root,
		"leaves",
		fmt.Sprintf("%02x", leafhash[0]),
		fmt.Sprintf("%02x", leafhash[1]),
		fmt.Sprintf("%02x", leafhash[2]),
		fmt.Sprintf("%0x", leafhash[3:]),
	}
	d := filepath.Join(frag[:5]...)
	return d, frag[5]
}

// TilePath builds the directory path and relative filename for the subtree tile with the
// given level and index.
// partialTileSize should be set to a non-zero number if the path to a partial tile
// is required.
func TilePath(root string, level, index, partialTileSize uint64) (string, string) {
	suffix := ""
	if partialTileSize > 0 {
		suffix = fmt.Sprintf(".%02x", partialTileSize)
	}

	frag := []string{
		root,
		"tile",
		fmt.Sprintf("%02x", level),
		fmt.Sprintf("%04x", (index >> 24)),
		fmt.Sprintf("%02x", (index>>16)&0xff),
		fmt.Sprintf("%02x", (index>>8)&0xff),
		fmt.Sprintf("%02x%s", index&0xff, suffix),
	}
	d := filepath.Join(frag[:6]...)
	return d, frag[6]
}
