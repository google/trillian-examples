package slots

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian-examples/witness/golang/omniwitness/usbarmory/internal/storage/testonly"
)

func TestOpenPartition(t *testing.T) {
	type slotGeo struct {
		Start  uint
		Length uint
	}
	toSlotGeo := func(in []Slot) []slotGeo {
		r := make([]slotGeo, len(in))
		for i := range in {
			r[i] = slotGeo{
				Start:  in[i].journal.start,
				Length: in[i].journal.length,
			}
		}
		return r
	}

	const devBlockSize = 32

	for _, test := range []struct {
		name      string
		geo       Geometry
		wantErr   bool
		wantSlots []slotGeo
	}{
		{
			name: "free space remaining",
			geo: Geometry{
				Start:       10,
				Length:      10,
				SlotLengths: []uint{1, 1, 2, 4},
			},
			wantSlots: []slotGeo{
				{Start: 10, Length: 1},
				{Start: 11, Length: 1},
				{Start: 12, Length: 2},
				{Start: 14, Length: 4},
			},
		}, {
			name: "fully allocated",
			geo: Geometry{
				Start:       10,
				Length:      10,
				SlotLengths: []uint{1, 1, 2, 4, 2},
			},
			wantSlots: []slotGeo{
				{Start: 10, Length: 1},
				{Start: 11, Length: 1},
				{Start: 12, Length: 2},
				{Start: 14, Length: 4},
				{Start: 18, Length: 2},
			},
		}, {
			name: "over allocated",
			geo: Geometry{
				Start:       10,
				Length:      10,
				SlotLengths: []uint{1, 1, 2, 4, 3},
			},
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dev := testonly.NewMemDev(t, devBlockSize)
			p, err := OpenPartition(dev, test.geo, sha256Func)
			if gotErr := err != nil; gotErr != test.wantErr {
				t.Fatalf("Got %v, wantErr %t", err, test.wantErr)
			}
			if test.wantErr {
				return
			}
			if diff := cmp.Diff(toSlotGeo(p.slots), test.wantSlots); diff != "" {
				t.Fatalf("Got diff: %s", diff)
			}
		})
	}
}

func memPartition(t *testing.T) (*Partition, testonly.MemDev) {
	t.Helper()
	md := testonly.NewMemDev(t, 32)
	geo := Geometry{
		Start:       10,
		Length:      10,
		SlotLengths: []uint{1, 1, 2, 4},
	}
	p, err := OpenPartition(md, geo, sha256Func)
	if err != nil {
		t.Fatalf("Failed to create mem partition: %v", err)
	}
	return p, md
}

func TestOpenSlot(t *testing.T) {
	p, _ := memPartition(t)
	for _, test := range []struct {
		name    string
		slot    uint
		wantErr bool
	}{
		{
			name: "works",
			slot: 0,
		}, {
			name:    "invalid slot: too big",
			slot:    uint(len(p.slots)),
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := p.Open(test.slot)
			if gotErr := err != nil; gotErr != test.wantErr {
				t.Fatalf("Failed to open slot: %v", err)
			}
		})
	}
}
