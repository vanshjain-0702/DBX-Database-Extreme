package engine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dbx/dbx/internal/protocol"
	"github.com/dbx/dbx/internal/util"
)

// StringStore provides string operations on the KVStore.
type StringStore struct{ kv *KVStore }

// NewStringStore creates a string store backed by kv.
func NewStringStore(kv *KVStore) *StringStore { return &StringStore{kv: kv} }

// Get returns the string value of key, or nil if not found.
func (s *StringStore) Get(key string) ([]byte, error) {
	e := s.kv.Get(key)
	if e == nil {
		return nil, nil
	}
	if e.Type != protocol.TypeString {
		return nil, util.ErrWrongType
	}
	return e.Value.([]byte), nil
}

// Set stores key as a string. Accepts optional EX/PX/NX/XX flags.
func (s *StringStore) Set(key string, value []byte, ttlSeconds int64, nx, xx bool) (bool, error) {
	if nx {
		if s.kv.Exists(key) {
			return false, nil
		}
	}
	if xx {
		if !s.kv.Exists(key) {
			return false, nil
		}
	}
	ttlNano := ttlSeconds * int64(1e9)
	s.kv.Set(key, value, protocol.TypeString, ttlNano)
	return true, nil
}

// GetSet atomically sets key to value and returns the old value.
func (s *StringStore) GetSet(key string, value []byte) ([]byte, error) {
	old, err := s.Get(key)
	if err != nil {
		return nil, err
	}
	s.kv.Set(key, value, protocol.TypeString, 0)
	return old, nil
}

// Incr increments key by 1. Creates key with value 1 if it doesn't exist.
func (s *StringStore) Incr(key string, by int64) (int64, error) {
	e, unlock := s.kv.GetForWrite(key)
	defer unlock()
	var n int64
	if e != nil {
		if e.Type != protocol.TypeString {
			return 0, util.ErrWrongType
		}
		v, err := strconv.ParseInt(string(e.Value.([]byte)), 10, 64)
		if err != nil {
			return 0, util.ErrOutOfRange
		}
		n = v + by
		e.Value = []byte(strconv.FormatInt(n, 10))
		e.Version = s.kv.nextVersion()
	} else {
		n = by
		sh := s.kv.shard(key)
		sh.data[key] = &Entry{
			Value:   []byte(strconv.FormatInt(n, 10)),
			Type:    protocol.TypeString,
			Version: s.kv.nextVersion(),
		}
	}
	return n, nil
}

// Append appends value to key. Returns new length.
func (s *StringStore) Append(key string, value []byte) (int, error) {
	e, unlock := s.kv.GetForWrite(key)
	defer unlock()
	if e != nil {
		if e.Type != protocol.TypeString {
			return 0, util.ErrWrongType
		}
		existing := e.Value.([]byte)
		newVal := append(existing, value...)
		e.Value = newVal
		return len(newVal), nil
	}
	shard := s.kv.shard(key)
	shard.data[key] = &Entry{Value: value, Type: protocol.TypeString}
	return len(value), nil
}

// Strlen returns the length of the string stored at key.
func (s *StringStore) Strlen(key string) (int, error) {
	v, err := s.Get(key)
	if err != nil {
		return 0, err
	}
	return len(v), nil
}

// GetRange returns a substring of the stored string.
func (s *StringStore) GetRange(key string, start, end int) ([]byte, error) {
	v, err := s.Get(key)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return []byte{}, nil
	}
	l := len(v)
	if start < 0 {
		start = l + start
	}
	if end < 0 {
		end = l + end
	}
	if start < 0 {
		start = 0
	}
	if end >= l {
		end = l - 1
	}
	if start > end {
		return []byte{}, nil
	}
	return v[start : end+1], nil
}

// SetRange overwrites part of the string at offset.
func (s *StringStore) SetRange(key string, offset int, value []byte) (int, error) {
	e, unlock := s.kv.GetForWrite(key)
	defer unlock()
	var current []byte
	if e != nil {
		if e.Type != protocol.TypeString {
			return 0, util.ErrWrongType
		}
		current = e.Value.([]byte)
	}
	end := offset + len(value)
	if end > len(current) {
		extended := make([]byte, end)
		copy(extended, current)
		current = extended
	}
	copy(current[offset:], value)
	if e != nil {
		e.Value = current
	} else {
		sh := s.kv.shard(key)
		sh.data[key] = &Entry{Value: current, Type: protocol.TypeString}
	}
	return len(current), nil
}

// MGet returns values for multiple keys.
func (s *StringStore) MGet(keys []string) ([][]byte, error) {
	result := make([][]byte, len(keys))
	for i, key := range keys {
		v, err := s.Get(key)
		if err != nil {
			result[i] = nil
		} else {
			result[i] = v
		}
	}
	return result, nil
}

// MSet sets multiple key-value pairs atomically.
func (s *StringStore) MSet(pairs map[string][]byte) {
	for k, v := range pairs {
		s.kv.Set(k, v, protocol.TypeString, 0)
	}
}

// ParseSetArgs parses SET command arguments (value, EX, PX, NX, XX).
func ParseSetArgs(args [][]byte) (value []byte, ttlSec int64, nx, xx bool, err error) {
	if len(args) == 0 {
		err = fmt.Errorf("missing value")
		return
	}
	value = args[0]
	for i := 1; i < len(args); i++ {
		switch strings.ToUpper(string(args[i])) {
		case "EX":
			if i+1 >= len(args) {
				err = util.ErrSyntax
				return
			}
			i++
			n, e := strconv.ParseInt(string(args[i]), 10, 64)
			if e != nil || n <= 0 {
				err = util.ErrOutOfRange
				return
			}
			ttlSec = n
		case "PX":
			if i+1 >= len(args) {
				err = util.ErrSyntax
				return
			}
			i++
			n, e := strconv.ParseInt(string(args[i]), 10, 64)
			if e != nil || n <= 0 {
				err = util.ErrOutOfRange
				return
			}
			ttlSec = n / 1000
		case "NX":
			nx = true
		case "XX":
			xx = true
		default:
			err = util.ErrSyntax
			return
		}
	}
	return
}
