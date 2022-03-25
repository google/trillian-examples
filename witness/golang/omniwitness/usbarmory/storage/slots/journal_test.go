package slots

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian-examples/witness/golang/omniwitness/usbarmory/storage/testonly"
)

func magic0Hdr() [4]byte {
	return [4]byte{magic0[0], magic0[1], magic0[2], magic0[3]}
}

func TestEntryMarshal(t *testing.T) {
	for _, test := range []struct {
		name    string
		e       *entry
		want    []byte
		wantErr bool
	}{
		{
			name: "golden",
			e: &entry{
				Magic:      magic0Hdr(),
				Revision:   42,
				DataLen:    uint64(len("hello")),
				DataSHA256: sha256.Sum256([]byte("hello")),
				Data:       []byte("hello"),
			},
			want: []byte{
				'T', 'F', 'J', '0',
				0x00, 0x00, 0x00, 0x2a,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x05,
				0x2c, 0xf2, 0x4d, 0xba, 0x5f, 0xb0, 0xa3, 0x0e, 0x26, 0xe8, 0x3b, 0x2a, 0xc5, 0xb9, 0xe2, 0x9e, 0x1b, 0x16, 0x1e, 0x5c, 0x1f, 0xa7, 0x42, 0x5e, 0x73, 0x04, 0x33, 0x62, 0x93, 0x8b, 0x98, 0x24,
				'h', 'e', 'l', 'l', 'o',
			},
		}, {
			name: "bad magic",
			e: &entry{
				Magic:      [4]byte{'n', 'O', 'p', 'E'},
				Revision:   42,
				DataLen:    uint64(len("hello")),
				DataSHA256: sha256.Sum256([]byte("hello")),
				Data:       []byte("hello"),
			},
			wantErr: true,
		}, {
			name: "bad hash",
			e: &entry{
				Magic:      magic0Hdr(),
				Revision:   42,
				DataLen:    uint64(len("hello")),
				DataSHA256: sha256.Sum256([]byte("nOpE")),
				Data:       []byte("hello"),
			},
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			b := &bytes.Buffer{}
			err := marshalEntry(test.e, b)
			if gotErr := err != nil; gotErr != test.wantErr {
				t.Fatalf("marshalEntry: %v, wantErr %t", err, test.wantErr)
			}
			if test.wantErr {
				return
			}

			if diff := cmp.Diff(b.Bytes(), test.want); diff != "" {
				t.Fatalf("Got diff: %v", diff)
			}
		})
	}
}

func TestEntryUnmarshal(t *testing.T) {
	for _, test := range []struct {
		name    string
		want    *entry
		b       []byte
		wantErr bool
	}{
		{
			name: "golden",
			want: &entry{
				Magic:      magic0Hdr(),
				Revision:   42,
				DataLen:    uint64(len("hello")),
				DataSHA256: sha256.Sum256([]byte("hello")),
				Data:       []byte("hello"),
			},
			b: []byte{
				'T', 'F', 'J', '0',
				0x00, 0x00, 0x00, 0x2a,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x05,
				0x2c, 0xf2, 0x4d, 0xba, 0x5f, 0xb0, 0xa3, 0x0e, 0x26, 0xe8, 0x3b, 0x2a, 0xc5, 0xb9, 0xe2, 0x9e, 0x1b, 0x16, 0x1e, 0x5c, 0x1f, 0xa7, 0x42, 0x5e, 0x73, 0x04, 0x33, 0x62, 0x93, 0x8b, 0x98, 0x24,
				'h', 'e', 'l', 'l', 'o',
			},
		}, {
			name: "bad magic",
			b: []byte{
				'N', 'o', 'P', 'e',
				0x00, 0x00, 0x00, 0x2a,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x05,
				0x2c, 0xf2, 0x4d, 0xba, 0x5f, 0xb0, 0xa3, 0x0e, 0x26, 0xe8, 0x3b, 0x2a, 0xc5, 0xb9, 0xe2, 0x9e, 0x1b, 0x16, 0x1e, 0x5c, 0x1f, 0xa7, 0x42, 0x5e, 0x73, 0x04, 0x33, 0x62, 0x93, 0x8b, 0x98, 0x24,
				'h', 'e', 'l', 'l', 'o',
			},
			wantErr: true,
		}, {
			name: "bad hash",
			b: []byte{
				'N', 'o', 'P', 'e',
				0x00, 0x00, 0x00, 0x2a,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x05,
				0x2c, 0xf2, 0x4d, 0xba, 0x5f, 0xb0, 0xa3, 0x0e, 0x26, 0xe8, 0x3b, 0x2a, 0xc5, 0xb9, 0xe2, 0x9e, 0x1b, 0x16, 0x1e, 0x5c, 0x1f, 0xa7, 0x42, 0x5e, 0x73, 0x04, 0x33, 0x62, 0x93, 0x8b, 0x98, 0x24,
				'W', 'R', 'O', 'N', 'G',
			},
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := unmarshalEntry(bytes.NewReader(test.b))
			if gotErr := err != nil; gotErr != test.wantErr {
				t.Fatalf("unmarshalEntry: %v, wantErr %t", err, test.wantErr)
			}
			if test.wantErr {
				return
			}

			if diff := cmp.Diff(got, test.want); diff != "" {
				t.Fatalf("Got diff: %v", diff)
			}
		})
	}
}

func TestOpenJournal(t *testing.T) {
	md := testonly.NewMemDev(t, 1)
	start, length := uint(1), uint(len(md)-2)
	j, err := OpenJournal(md, start, length)
	if err != nil {
		t.Fatalf("OpenJournal: %v", err)
	}
	if j.start != start {
		t.Fatalf("Journal.start=%d, want %d", j.start, start)
	}
	if j.length != length {
		t.Fatalf("Journal.length=%d, want %d", j.length, length)
	}
}

func TestWriteSizeLimit(t *testing.T) {
	storageBlocks := uint(20)
	md := testonly.NewMemDev(t, storageBlocks)
	start, length := uint(1), uint(storageBlocks)

	j, err := OpenJournal(md, start, length)
	if err != nil {
		t.Fatalf("OpenJournal: %v", err)
	}

	limit := int((storageBlocks * md.BlockSize() / 3) - entryHeaderSize)
	if err := j.Update(fill(limit, "ok...")); err != nil {
		t.Fatalf("Update: %v, but expected write to succeed", err)
	}
	if err := j.Update(fill(limit+1, "BOOM")); err == nil {
		t.Fatal("Update suceeded, but expected write fail")
	}
}

func TestRoundTrip(t *testing.T) {
	storageBlocks := uint(20)
	md := testonly.NewMemDev(t, storageBlocks)
	start, length := uint(1), uint(storageBlocks)

	var prevData []byte
	for i, test := range []struct {
		data               []byte
		expectedWriteBlock uint
	}{
		{data: []byte{}, expectedWriteBlock: start},                                        // 1 block
		{data: fill(256, "hello"), expectedWriteBlock: start + 1},                          // 1 block
		{data: fill(1000, "there"), expectedWriteBlock: start + 1 + 1},                     // 3 blocks
		{data: fill(30, "how"), expectedWriteBlock: start + 1 + 1 + 3},                     // 1 block
		{data: fill(3000, "are"), expectedWriteBlock: start + 1 + 1 + 3 + 1},               // 6 blocks
		{data: fill(59, "you"), expectedWriteBlock: start + 1 + 1 + 3 + 1 + 6},             // 1 block
		{data: fill(3000, "doing"), expectedWriteBlock: start + 1 + 1 + 3 + 1 + 6 + 1},     // 6 blocks,
		{data: fill(1000, "doing"), expectedWriteBlock: start + 1 + 1 + 3 + 1 + 6 + 1 + 6}, // 3 blocks, won't fit so will end up writing at `start`
		{data: fill(3000, "today?"), expectedWriteBlock: start + 3},                        // 6 blocks
		{data: fill(20, "All done!"), expectedWriteBlock: start + 9},                       // 1 blocks
	} {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			j, err := OpenJournal(md, start, length)
			if err != nil {
				t.Fatalf("OpenJournal: %v", err)
			}

			// Check the current record is as we expect:
			// - Revision should match the iteration count
			// - The next block to write to is correct
			// - The stored data matches what we last wrote
			curData, rev := j.Data()
			if rev != uint32(i) {
				t.Errorf("Got revision %d, want %d", rev, i)
			}
			if got, want := j.nextBlock, test.expectedWriteBlock; got != want {
				t.Errorf("nextBlock = %d, want %d", got, want)
			}
			// Ensure we see the data written in the last iteration, if any
			if prevData != nil && !bytes.Equal(curData, prevData) {
				t.Errorf("Got data %q, want %q", string(curData), string(prevData))
			}
			prevData = test.data

			// Write some updated data
			if err := j.Update(test.data); err != nil {
				t.Fatalf("Update: %v", err)
			}
		})
	}
}

// fill returns a slice of length n containing as many repeats of s as necessary
// to fill it (including partial at the end if needed).
func fill(n int, s string) []byte {
	return []byte(strings.Repeat(s, n/len(s)+1))[:n]
}
