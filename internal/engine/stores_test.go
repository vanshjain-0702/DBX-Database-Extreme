package engine

import (
	"testing"

	"github.com/dbx/dbx/internal/protocol"
)

func TestTypedStoresCoverCommonMutations(t *testing.T) {
	kv := New(8)
	str := NewStringStore(kv)
	if _, err := str.Set("k", []byte("1"), 0, false, false); err != nil {
		t.Fatal(err)
	}
	if got, err := str.Get("k"); err != nil || string(got) != "1" {
		t.Fatalf("%q %v", got, err)
	}
	if n, err := str.Incr("n", 2); err != nil || n != 2 {
		t.Fatal(n, err)
	}
	if n, err := str.Append("k", []byte("x")); err != nil || n != 2 {
		t.Fatal(n, err)
	}
	hash := NewHashStore(kv)
	if _, err := hash.HSet("h", map[string][]byte{"f": []byte("v")}); err != nil {
		t.Fatal(err)
	}
	if got, err := hash.HGet("h", "f"); err != nil || string(got) != "v" {
		t.Fatal(got, err)
	}
	list := NewListStore(kv)
	if _, err := list.LPush("l", [][]byte{[]byte("a")}); err != nil {
		t.Fatal(err)
	}
	set := NewSetStore(kv)
	if _, err := set.SAdd("s", [][]byte{[]byte("m")}); err != nil {
		t.Fatal(err)
	}
	zset := NewZSetStore(kv)
	if _, err := zset.ZAdd("z", map[string]float64{"m": 1}); err != nil {
		t.Fatal(err)
	}
	kv.Set("ttl", []byte("x"), protocol.TypeString, 1)
	_ = kv.Expire("ttl", 1)
	_ = kv.TTL("ttl")
	_ = kv.Persist("ttl")
	_ = kv.Keys("*")
	_ = kv.DBSize()
	_ = kv.Type("k")
	_ = kv.Exists("k")
	kv.Delete("ttl")
	if err := kv.Rename("k", "k2"); err != nil {
		t.Fatal(err)
	}
	_ = kv.KeyspaceStats()
	_ = kv.MemoryUsage()
	_ = kv.Snapshot()
	_ = kv.ExpiredKeys()
	_ = kv.DeleteExpiredLimit(8)
	hash.HExists("h", "f")
	_, _ = hash.HLen("h")
	_, _ = list.LLen("l")
	_, _ = set.SCard("s")
	_, _ = zset.ZCard("z")
	geo := NewGeoStore(kv)
	if _, err := geo.GeoAdd("g", []GeoPoint{{Name: "a", Longitude: -73.9, Latitude: 40.7}}); err != nil {
		t.Fatal(err)
	}
	if _, err := geo.GeoPos("g", []string{"a"}); err != nil {
		t.Fatal(err)
	}
	bits := NewBitmapStore(kv)
	if _, err := bits.SetBit("b", 3, 1); err != nil {
		t.Fatal(err)
	}
	if got, err := bits.GetBit("b", 3); err != nil || got != 1 {
		t.Fatal(got, err)
	}
	doc := NewDocumentStore(kv)
	if err := doc.Set("d", map[string]interface{}{"n": 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Get("d"); err != nil {
		t.Fatal(err)
	}
	if err := doc.SetPath("d", "n", 2); err != nil {
		t.Fatal(err)
	}
	kv.FlushAll()
}
