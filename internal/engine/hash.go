package engine

import (
	"strconv"

	"github.com/dbx/dbx/internal/protocol"
	"github.com/dbx/dbx/internal/util"
)

// hashMap is the underlying type for hash entries.
type hashMap map[string][]byte

// HashStore provides hash operations.
type HashStore struct{ kv *KVStore }

func NewHashStore(kv *KVStore) *HashStore { return &HashStore{kv: kv} }

func (h *HashStore) getOrCreate(key string) (hashMap, func(), error) {
	e, unlock := h.kv.GetForWrite(key)
	if e == nil {
		// Create new hash
		m := make(hashMap)
		sh := h.kv.shard(key)
		sh.data[key] = &Entry{Value: m, Type: protocol.TypeHash}
		return m, unlock, nil
	}
	if e.Type != protocol.TypeHash {
		unlock()
		return nil, func() {}, util.ErrWrongType
	}
	return e.Value.(hashMap), unlock, nil
}

func (h *HashStore) getReadOnly(key string) (hashMap, func(), error) {
	e, unlock := h.kv.GetForRead(key)
	if e == nil {
		return nil, func(){}, nil
	}
	if e.Type != protocol.TypeHash {
		unlock()
		return nil, func(){}, util.ErrWrongType
	}
	return e.Value.(hashMap), unlock, nil
}

// HSet sets field(s) in key. Returns number of new fields added.
func (h *HashStore) HSet(key string, pairs map[string][]byte) (int, error) {
	m, unlock, err := h.getOrCreate(key)
	defer unlock()
	if err != nil {
		return 0, err
	}
	added := 0
	for f, v := range pairs {
		if _, exists := m[f]; !exists {
			added++
		}
		m[f] = v
	}
	return added, nil
}

// HGet returns the value of field in key.
func (h *HashStore) HGet(key, field string) ([]byte, error) {
	m, unlock, err := h.getReadOnly(key)
	defer unlock()
	if err != nil || m == nil {
		return nil, err
	}
	return m[field], nil
}

// HMGet returns multiple field values.
func (h *HashStore) HMGet(key string, fields []string) ([][]byte, error) {
	m, unlock, err := h.getReadOnly(key)
	defer unlock()
	result := make([][]byte, len(fields))
	if err != nil {
		return result, err
	}
	for i, f := range fields {
		if m != nil {
			result[i] = m[f]
		}
	}
	return result, nil
}

// HDel deletes fields from key. Returns number deleted.
func (h *HashStore) HDel(key string, fields []string) (int, error) {
	m, unlock, err := h.getOrCreate(key)
	defer unlock()
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, f := range fields {
		if _, ok := m[f]; ok {
			delete(m, f)
			deleted++
		}
	}
	return deleted, nil
}

// HGetAll returns all field-value pairs.
func (h *HashStore) HGetAll(key string) (map[string][]byte, error) {
	m, unlock, err := h.getReadOnly(key)
	defer unlock()
	if err != nil || m == nil {
		return nil, err
	}
	result := make(map[string][]byte, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result, nil
}

// HKeys returns all fields.
func (h *HashStore) HKeys(key string) ([]string, error) {
	m, unlock, err := h.getReadOnly(key)
	defer unlock()
	if err != nil || m == nil {
		return nil, err
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys, nil
}

// HVals returns all values.
func (h *HashStore) HVals(key string) ([][]byte, error) {
	m, unlock, err := h.getReadOnly(key)
	defer unlock()
	if err != nil || m == nil {
		return nil, err
	}
	vals := make([][]byte, 0, len(m))
	for _, v := range m {
		vals = append(vals, v)
	}
	return vals, nil
}

// HLen returns the number of fields.
func (h *HashStore) HLen(key string) (int, error) {
	m, unlock, err := h.getReadOnly(key)
	defer unlock()
	return len(m), err
}

// HExists returns true if field exists in key.
func (h *HashStore) HExists(key, field string) (bool, error) {
	m, unlock, err := h.getReadOnly(key)
	defer unlock()
	if err != nil || m == nil {
		return false, err
	}
	_, ok := m[field]
	return ok, nil
}

// HIncrBy increments field by delta.
func (h *HashStore) HIncrBy(key, field string, delta int64) (int64, error) {
	m, unlock, err := h.getOrCreate(key)
	defer unlock()
	if err != nil {
		return 0, err
	}
	var n int64
	if v, ok := m[field]; ok {
		n, err = strconv.ParseInt(string(v), 10, 64)
		if err != nil {
			return 0, util.ErrOutOfRange
		}
	}
	n += delta
	m[field] = []byte(strconv.FormatInt(n, 10))
	return n, nil
}
