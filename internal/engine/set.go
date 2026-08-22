package engine

import (
	"math/rand"

	"github.com/dbx/dbx/internal/protocol"
	"github.com/dbx/dbx/internal/util"
)

// dbxSet is the underlying set type.
type dbxSet map[string]struct{}

// SetStore provides set operations.
type SetStore struct{ kv *KVStore }

func NewSetStore(kv *KVStore) *SetStore { return &SetStore{kv: kv} }

func (s *SetStore) getOrCreate(key string) (dbxSet, func(), error) {
	e, unlock := s.kv.GetForWrite(key)
	if e == nil {
		st := make(dbxSet)
		sh := s.kv.shard(key)
		sh.data[key] = &Entry{Value: st, Type: protocol.TypeSet}
		return st, unlock, nil
	}
	if e.Type != protocol.TypeSet {
		unlock()
		return nil, func() {}, util.ErrWrongType
	}
	return e.Value.(dbxSet), unlock, nil
}

func (s *SetStore) getReadOnly(key string) (dbxSet, func(), error) {
	e, unlock := s.kv.GetForRead(key)
	if e == nil {
		return nil, func(){}, nil
	}
	if e.Type != protocol.TypeSet {
		unlock()
		return nil, func(){}, util.ErrWrongType
	}
	return e.Value.(dbxSet), unlock, nil
}

// SAdd adds members. Returns count added.
func (s *SetStore) SAdd(key string, members [][]byte) (int, error) {
	st, unlock, err := s.getOrCreate(key)
	defer unlock()
	if err != nil {
		return 0, err
	}
	added := 0
	for _, m := range members {
		k := string(m)
		if _, ok := st[k]; !ok {
			st[k] = struct{}{}
			added++
		}
	}
	return added, nil
}

// SRem removes members. Returns count removed.
func (s *SetStore) SRem(key string, members [][]byte) (int, error) {
	st, unlock, err := s.getOrCreate(key)
	defer unlock()
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, m := range members {
		k := string(m)
		if _, ok := st[k]; ok {
			delete(st, k)
			removed++
		}
	}
	return removed, nil
}

// SMembers returns all members.
func (s *SetStore) SMembers(key string) ([][]byte, error) {
	st, unlock, err := s.getReadOnly(key)
	defer unlock()
	if err != nil || st == nil {
		return nil, err
	}
	result := make([][]byte, 0, len(st))
	for m := range st {
		result = append(result, []byte(m))
	}
	return result, nil
}

// SIsMember returns true if member is in set.
func (s *SetStore) SIsMember(key string, member []byte) (bool, error) {
	st, unlock, err := s.getReadOnly(key)
	defer unlock()
	if err != nil || st == nil {
		return false, err
	}
	_, ok := st[string(member)]
	return ok, nil
}

// SCard returns cardinality of the set.
func (s *SetStore) SCard(key string) (int, error) {
	st, unlock, err := s.getReadOnly(key)
	defer unlock()
	return len(st), err
}

// SInter returns intersection of multiple sets.
func (s *SetStore) SInter(keys []string) ([][]byte, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	first, unlock, err := s.getReadOnly(keys[0])
	defer unlock()
	if err != nil || first == nil {
		return nil, err
	}
	result := make(dbxSet)
	for m := range first {
		result[m] = struct{}{}
	}
	for _, k := range keys[1:] {
		st, unlock, err := s.getReadOnly(k)
		defer unlock()
		if err != nil {
			return nil, err
		}
		for m := range result {
			if _, ok := st[m]; !ok {
				delete(result, m)
			}
		}
	}
	out := make([][]byte, 0, len(result))
	for m := range result {
		out = append(out, []byte(m))
	}
	return out, nil
}

// SUnion returns union of multiple sets.
func (s *SetStore) SUnion(keys []string) ([][]byte, error) {
	result := make(dbxSet)
	for _, k := range keys {
		st, unlock, err := s.getReadOnly(k)
		defer unlock()
		if err != nil {
			return nil, err
		}
		for m := range st {
			result[m] = struct{}{}
		}
	}
	out := make([][]byte, 0, len(result))
	for m := range result {
		out = append(out, []byte(m))
	}
	return out, nil
}

// SDiff returns difference (first set minus all others).
func (s *SetStore) SDiff(keys []string) ([][]byte, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	first, unlock, err := s.getReadOnly(keys[0])
	defer unlock()
	if err != nil || first == nil {
		return nil, err
	}
	result := make(dbxSet)
	for m := range first {
		result[m] = struct{}{}
	}
	for _, k := range keys[1:] {
		st, unlock, err := s.getReadOnly(k)
		defer unlock()
		if err != nil {
			return nil, err
		}
		for m := range st {
			delete(result, m)
		}
	}
	out := make([][]byte, 0, len(result))
	for m := range result {
		out = append(out, []byte(m))
	}
	return out, nil
}

// SRandMember returns count random members (without removal).
func (s *SetStore) SRandMember(key string, count int) ([][]byte, error) {
	st, unlock, err := s.getReadOnly(key)
	defer unlock()
	if err != nil || st == nil {
		return nil, err
	}
	members := make([]string, 0, len(st))
	for m := range st {
		members = append(members, m)
	}
	rand.Shuffle(len(members), func(i, j int) { members[i], members[j] = members[j], members[i] })
	if count > len(members) {
		count = len(members)
	}
	result := make([][]byte, count)
	for i, m := range members[:count] {
		result[i] = []byte(m)
	}
	return result, nil
}

// SPop removes and returns count random members.
func (s *SetStore) SPop(key string, count int) ([][]byte, error) {
	st, unlock, err := s.getOrCreate(key)
	defer unlock()
	if err != nil {
		return nil, err
	}
	members := make([]string, 0, len(st))
	for m := range st {
		members = append(members, m)
	}
	rand.Shuffle(len(members), func(i, j int) { members[i], members[j] = members[j], members[i] })
	if count > len(members) {
		count = len(members)
	}
	result := make([][]byte, count)
	for i, m := range members[:count] {
		result[i] = []byte(m)
		delete(st, m)
	}
	return result, nil
}
