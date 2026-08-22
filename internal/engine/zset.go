package engine

import (
	"fmt"
	"math"
	"sort"

	"github.com/dbx/dbx/internal/protocol"
	"github.com/dbx/dbx/internal/util"
)

// zsetMember is a member with a score.
type zsetMember struct {
	Member string
	Score  float64
}

// dbxZSet stores sorted set data.
type dbxZSet struct {
	scores  map[string]float64
	sorted  []zsetMember // maintained in sorted order
}

func newZSet() *dbxZSet {
	return &dbxZSet{scores: make(map[string]float64)}
}

func (z *dbxZSet) insertSorted(member string, score float64) {
	// Binary search insertion into sorted slice
	i := sort.Search(len(z.sorted), func(i int) bool {
		if z.sorted[i].Score != score {
			return z.sorted[i].Score > score
		}
		return z.sorted[i].Member >= member
	})
	z.sorted = append(z.sorted, zsetMember{})
	copy(z.sorted[i+1:], z.sorted[i:])
	z.sorted[i] = zsetMember{Member: member, Score: score}
}

func (z *dbxZSet) removeSorted(member string, score float64) {
	i := sort.Search(len(z.sorted), func(i int) bool {
		if z.sorted[i].Score != score {
			return z.sorted[i].Score > score
		}
		return z.sorted[i].Member >= member
	})
	if i < len(z.sorted) && z.sorted[i].Member == member {
		z.sorted = append(z.sorted[:i], z.sorted[i+1:]...)
	}
}

// ZSetStore provides sorted set operations.
type ZSetStore struct{ kv *KVStore }

func NewZSetStore(kv *KVStore) *ZSetStore { return &ZSetStore{kv: kv} }

func (z *ZSetStore) getOrCreate(key string) (*dbxZSet, func(), error) {
	e, unlock := z.kv.GetForWrite(key)
	if e == nil {
		zs := newZSet()
		sh := z.kv.shard(key)
		sh.data[key] = &Entry{Value: zs, Type: protocol.TypeZSet}
		return zs, unlock, nil
	}
	if e.Type != protocol.TypeZSet {
		unlock()
		return nil, func() {}, util.ErrWrongType
	}
	return e.Value.(*dbxZSet), unlock, nil
}

func (z *ZSetStore) getReadOnly(key string) (*dbxZSet, func(), error) {
	e, unlock := z.kv.GetForRead(key)
	if e == nil {
		return nil, func(){}, nil
	}
	if e.Type != protocol.TypeZSet {
		unlock()
		return nil, func(){}, util.ErrWrongType
	}
	return e.Value.(*dbxZSet), unlock, nil
}

// ZAdd adds/updates members. Returns count added.
func (z *ZSetStore) ZAdd(key string, members map[string]float64) (int, error) {
	zs, unlock, err := z.getOrCreate(key)
	defer unlock()
	if err != nil {
		return 0, err
	}
	added := 0
	for member, score := range members {
		if oldScore, exists := zs.scores[member]; exists {
			zs.removeSorted(member, oldScore)
		} else {
			added++
		}
		zs.scores[member] = score
		zs.insertSorted(member, score)
	}
	return added, nil
}

// ZRem removes members. Returns count removed.
func (z *ZSetStore) ZRem(key string, members []string) (int, error) {
	zs, unlock, err := z.getOrCreate(key)
	defer unlock()
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, member := range members {
		if score, ok := zs.scores[member]; ok {
			delete(zs.scores, member)
			zs.removeSorted(member, score)
			removed++
		}
	}
	return removed, nil
}

// ZScore returns the score of member.
func (z *ZSetStore) ZScore(key, member string) (float64, bool, error) {
	zs, unlock, err := z.getReadOnly(key)
	defer unlock()
	if err != nil || zs == nil {
		return 0, false, err
	}
	score, ok := zs.scores[member]
	return score, ok, nil
}

// ZRank returns the rank (0-based) of member in ascending order.
func (z *ZSetStore) ZRank(key, member string) (int, bool, error) {
	zs, unlock, err := z.getReadOnly(key)
	defer unlock()
	if err != nil || zs == nil {
		return 0, false, err
	}
	for i, m := range zs.sorted {
		if m.Member == member {
			return i, true, nil
		}
	}
	return 0, false, nil
}

// ZRevRank returns rank in descending order.
func (z *ZSetStore) ZRevRank(key, member string) (int, bool, error) {
	rank, ok, err := z.ZRank(key, member)
	if err != nil || !ok {
		return 0, ok, err
	}
	zs, unlock, _ := z.getReadOnly(key)
	defer unlock()
	return len(zs.sorted) - 1 - rank, true, nil
}

// ZRange returns members in rank range [start, stop] (ascending).
func (z *ZSetStore) ZRange(key string, start, stop int, withScores bool) ([]zsetMember, error) {
	zs, unlock, err := z.getReadOnly(key)
	defer unlock()
	if err != nil || zs == nil {
		return nil, err
	}
	n := len(zs.sorted)
	if start < 0 {
		start = n + start
	}
	if stop < 0 {
		stop = n + stop
	}
	if start < 0 {
		start = 0
	}
	if stop >= n {
		stop = n - 1
	}
	if start > stop {
		return nil, nil
	}
	return append([]zsetMember{}, zs.sorted[start:stop+1]...), nil
}

// ZRangeByScore returns members with scores in [min, max].
func (z *ZSetStore) ZRangeByScore(key string, min, max float64) ([]zsetMember, error) {
	zs, unlock, err := z.getReadOnly(key)
	defer unlock()
	if err != nil || zs == nil {
		return nil, err
	}
	var result []zsetMember
	for _, m := range zs.sorted {
		if m.Score >= min && m.Score <= max {
			result = append(result, m)
		}
	}
	return result, nil
}

// ZCard returns cardinality of sorted set.
func (z *ZSetStore) ZCard(key string) (int, error) {
	zs, unlock, err := z.getReadOnly(key)
	defer unlock()
	if err != nil || zs == nil {
		return 0, err
	}
	return len(zs.scores), nil
}

// ZIncrBy increments member's score by delta.
func (z *ZSetStore) ZIncrBy(key, member string, delta float64) (float64, error) {
	zs, unlock, err := z.getOrCreate(key)
	defer unlock()
	if err != nil {
		return 0, err
	}
	score := zs.scores[member]
	if oldScore, ok := zs.scores[member]; ok {
		zs.removeSorted(member, oldScore)
	}
	score += delta
	zs.scores[member] = score
	zs.insertSorted(member, score)
	return score, nil
}

// ZCount returns count of members with scores in [min, max].
func (z *ZSetStore) ZCount(key string, min, max float64) (int, error) {
	members, err := z.ZRangeByScore(key, min, max)
	return len(members), err
}

// ParseInfScore parses "+inf" / "-inf" / numeric score strings.
func ParseInfScore(s string) (float64, error) {
	switch s {
	case "+inf":
		return math.Inf(1), nil
	case "-inf":
		return math.Inf(-1), nil
	}
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}
