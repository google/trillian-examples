package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"reflect"
	"time"

	"github.com/golang/groupcache/lru"
	"github.com/google/trillian"
	"github.com/google/trillian-examples/registers/trillian_client"
	"google.golang.org/grpc"
)

var (
	trillianLog = flag.String("trillian_log", "localhost:8090", "address of the Trillian Log RPC server.")
	logID       = flag.Int64("log_id", 0, "Trillian LogID to read.")
	trillianMap = flag.String("trillian_map", "localhost:8095", "address of the Trillian Map RPC server.")
	mapID       = flag.Int64("map_id", 0, "Trillian MapID to write.")
)

// Key types
const (
	KTRecord = "record:"
	KTKey    = "key:"
)

type record struct {
	Entry map[string]interface{}
	Items []map[string]interface{}
}

// add adds an item. Only adds if the item is not already present in Items.
func (r *record) add(i map[string]interface{}) {
	for _, ii := range r.Items {
		if reflect.DeepEqual(i, ii) {
			return
		}
	}
	r.Items = append(r.Items, i)
}

type mapInfo struct {
	mapID int64
	tc    trillian.TrillianMapClient
	ctx   context.Context
}

type recordCache struct {
	records *lru.Cache
	m       *mapInfo
}

func newCache(tc trillian.TrillianMapClient, mapID int64, max int, ctx context.Context) *recordCache {
	c := &recordCache{records: lru.New(max), m: &mapInfo{mapID: mapID, tc: tc, ctx: ctx}}
	c.records.OnEvicted = func(key lru.Key, value interface{}) { recordEvicted(c, key, value) }
	return c
}

func hash(kt string, key string) []byte {
	hash := sha256.Sum256([]byte(kt + key))
	return hash[:]
}

func recordHash(key string) []byte {
	return hash(KTRecord, key)
}

func keyHash(index int) []byte {
	return hash(KTKey, string(index))
}

func addToMap(m *mapInfo, h []byte, v []byte) {
	l := trillian.MapLeaf{
		Index:     h,
		LeafValue: v,
	}

	req := trillian.SetMapLeavesRequest{
		MapId:  m.mapID,
		Leaves: []*trillian.MapLeaf{&l},
	}

	_, err := m.tc.SetLeaves(m.ctx, &req)
	if err != nil {
		log.Fatalf("SetLeaves() failed: %v", err)
	}
}

var keyCount int

func addKey(m *mapInfo, key string) {
	addToMap(m, keyHash(keyCount), []byte(key))
	keyCount++
}

func recordEvicted(c *recordCache, key lru.Key, value interface{}) {
	fmt.Printf("evicting %v -> %v\n", key, value)

	v, err := json.Marshal(value)
	if err != nil {
		log.Fatalf("Marshal() failed: %v", err)
	}

	hash := recordHash(key.(string))
	addToMap(c.m, hash, v)
}

func (c *recordCache) getLeaf(key string) (*record, error) {
	hash := recordHash(key)
	index := [1][]byte{hash}
	req := &trillian.GetMapLeavesRequest{
		MapId: c.m.mapID,
		Index: index[:],
	}

	resp, err := c.m.tc.GetLeaves(c.m.ctx, req)
	if err != nil {
		return nil, err
	}

	l := resp.MapLeafInclusion[0].Leaf.LeafValue
	log.Printf("key=%v leaf=%s", key, l)
	// FIXME: we should be able to detect non-existent vs. empty leaves
	if len(l) == 0 {
		return nil, nil
	}

	var r record
	err = json.Unmarshal(l, &r)
	if err != nil {
		return nil, err
	}

	return &r, nil
}

// Get the current record for the given key, possibly going to Trillian to look it up, possibly flushing the cache if needed.
func (c *recordCache) get(key string) (*record, error) {
	r, ok := c.records.Get(key)
	if !ok {
		rr, err := c.getLeaf(key)
		if err != nil {
			return nil, err
		}
		c.cacheExisting(key, rr)
		r = rr
	}
	if r == nil {
		return nil, nil
	}
	return r.(*record), nil
}

// create creates a new entry or replaces the existing one.
func (c *recordCache) create(key string, entry map[string]interface{}, item map[string]interface{}) {
	c.records.Add(key, &record{Entry: entry, Items: []map[string]interface{}{item}})
}

func (c *recordCache) cacheExisting(key string, r *record) {
	c.records.Add(key, r)
}

func (c *recordCache) flush() {
	c.records.Clear()
}

type logScanner struct {
	cache *recordCache
}

func (s *logScanner) Leaf(leaf *trillian.LogLeaf) error {
	//log.Printf("leaf %d: %s", leaf.LeafIndex, leaf.LeafValue)
	var l map[string]interface{}
	err := json.Unmarshal(leaf.LeafValue, &l)
	if err != nil {
		return err
	}
	//log.Printf("%v", l)

	e := l["Entry"].(map[string]interface{})
	t, err := time.Parse(time.RFC3339, e["entry-timestamp"].(string))
	if err != nil {
		return err
	}

	k := e["key"].(string)
	i := l["Item"].(map[string]interface{})
	log.Printf("k: %s ts: %s", k, t)

	cr, err := s.cache.get(k)
	if err != nil {
		return err
	}
	if cr == nil {
		s.cache.create(k, e, i)
		addKey(s.cache.m, k)
		return nil
	}

	ct, err := time.Parse(time.RFC3339, cr.Entry["entry-timestamp"].(string))
	if err != nil {
		return err
	}

	if t.Before(ct) {
		log.Printf("Skip")
		return nil
	} else if t.After(ct) {
		log.Printf("Replace")
		s.cache.create(k, e, i)
		return nil
	}

	log.Printf("Add")
	cr.add(i)

	return nil
}

func main() {
	flag.Parse()

	tc := trillian_client.New(*trillianLog)
	defer tc.Close()

	g, err := grpc.Dial(*trillianMap, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to dial Trillian Log: %v", err)
	}
	tmc := trillian.NewTrillianMapClient(g)

	rc := newCache(tmc, *mapID, 3, context.Background())
	defer rc.flush()
	err = tc.Scan(*logID, &logScanner{cache: rc})
	if err != nil {
		log.Fatal(err)
	}
}
