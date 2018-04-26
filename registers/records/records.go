package records

// Key types
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"

	"github.com/google/trillian"
)

const (
	KTRecord = "record:"
	KTKey    = "key:"
)

func hash(kt string, key string) []byte {
	hash := sha256.Sum256([]byte(kt + key))
	return hash[:]
}

func RecordHash(key string) []byte {
	return hash(KTRecord, key)
}

func KeyHash(index int) []byte {
	return hash(KTKey, string(index))
}

func GetValue(tmc trillian.TrillianMapClient, id int64, hash []byte) *string {
	index := [1][]byte{hash}
	req := &trillian.GetMapLeavesRequest{
		MapId: id,
		Index: index[:],
	}

	resp, err := tmc.GetLeaves(context.Background(), req)
	if err != nil {
		log.Fatalf("Can't get leaf '%s': %v", hex.EncodeToString(hash), err)
	}
	if resp.MapLeafInclusion[0].Leaf.LeafValue == nil {
		return nil
	}
	s := string(resp.MapLeafInclusion[0].Leaf.LeafValue)
	return &s
}
