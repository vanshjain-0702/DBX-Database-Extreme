package util

import "testing"

func TestMurmurHash32MatchesMurmur3X86(t *testing.T) {
	// Values from spaolacci/murmur3 v1.1.0 Sum32 (seed 0). Include 4-byte
	// keys — that length made the old unsafe implementation trip checkptr.
	cases := map[string]uint32{
		"":      0,
		"a":     1009084850,
		"ab":    2613040991,
		"abc":   3017643002,
		"abcd":  1139631978,
		"hello": 613153351,
		"SET":   2384292522,
		"k":     3485312465,
		"n":     3327252652,
	}
	for key, want := range cases {
		if got := MurmurHash32(key); got != want {
			t.Errorf("MurmurHash32(%q) = %d, want %d", key, got, want)
		}
	}
}

func TestShardIndexIsStable(t *testing.T) {
	if ShardIndex("user:42", 16) != int(MurmurHash32("user:42"))%16 {
		t.Fatal("shard mapping drifted")
	}
	if ShardIndex("x", 0) != 0 {
		t.Fatal("zero shards should map to 0")
	}
}
