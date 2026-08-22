package engine

import (
	"github.com/dbx/dbx/internal/protocol"
	"github.com/dbx/dbx/internal/util"
)

// dbxList is the underlying list type.
type dbxList struct {
	items [][]byte
}

// ListStore provides list operations.
type ListStore struct{ kv *KVStore }

func NewListStore(kv *KVStore) *ListStore { return &ListStore{kv: kv} }

func (l *ListStore) getOrCreate(key string) (*dbxList, func(), error) {
	e, unlock := l.kv.GetForWrite(key)
	if e == nil {
		lst := &dbxList{}
		sh := l.kv.shard(key)
		sh.data[key] = &Entry{Value: lst, Type: protocol.TypeList}
		return lst, unlock, nil
	}
	if e.Type != protocol.TypeList {
		unlock()
		return nil, func() {}, util.ErrWrongType
	}
	return e.Value.(*dbxList), unlock, nil
}

func (l *ListStore) getReadOnly(key string) (*dbxList, func(), error) {
	e, unlock := l.kv.GetForRead(key)
	if e == nil {
		return nil, func(){}, nil
	}
	if e.Type != protocol.TypeList {
		unlock()
		return nil, func(){}, util.ErrWrongType
	}
	return e.Value.(*dbxList), unlock, nil
}

// LPush prepends values to list. Returns new length.
func (l *ListStore) LPush(key string, values [][]byte) (int, error) {
	lst, unlock, err := l.getOrCreate(key)
	defer unlock()
	if err != nil {
		return 0, err
	}
	for _, v := range values {
		lst.items = append([][]byte{v}, lst.items...)
	}
	return len(lst.items), nil
}

// RPush appends values to list. Returns new length.
func (l *ListStore) RPush(key string, values [][]byte) (int, error) {
	lst, unlock, err := l.getOrCreate(key)
	defer unlock()
	if err != nil {
		return 0, err
	}
	lst.items = append(lst.items, values...)
	return len(lst.items), nil
}

// LPop removes and returns count elements from the head.
func (l *ListStore) LPop(key string, count int) ([][]byte, error) {
	lst, unlock, err := l.getOrCreate(key)
	defer unlock()
	if err != nil {
		return nil, err
	}
	if len(lst.items) == 0 {
		return nil, nil
	}
	if count > len(lst.items) {
		count = len(lst.items)
	}
	result := make([][]byte, count)
	copy(result, lst.items[:count])
	lst.items = lst.items[count:]
	return result, nil
}

// RPop removes and returns count elements from the tail.
func (l *ListStore) RPop(key string, count int) ([][]byte, error) {
	lst, unlock, err := l.getOrCreate(key)
	defer unlock()
	if err != nil {
		return nil, err
	}
	if len(lst.items) == 0 {
		return nil, nil
	}
	if count > len(lst.items) {
		count = len(lst.items)
	}
	start := len(lst.items) - count
	result := make([][]byte, count)
	copy(result, lst.items[start:])
	lst.items = lst.items[:start]
	return result, nil
}

// LRange returns a slice of the list.
func (l *ListStore) LRange(key string, start, stop int) ([][]byte, error) {
	lst, unlock, err := l.getReadOnly(key)
	defer unlock()
	if err != nil || lst == nil {
		return nil, err
	}
	n := len(lst.items)
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
		return [][]byte{}, nil
	}
	result := make([][]byte, stop-start+1)
	copy(result, lst.items[start:stop+1])
	return result, nil
}

// LLen returns the length of the list.
func (l *ListStore) LLen(key string) (int, error) {
	lst, unlock, err := l.getReadOnly(key)
	defer unlock()
	if err != nil || lst == nil {
		return 0, err
	}
	return len(lst.items), nil
}

// LIndex returns element at index.
func (l *ListStore) LIndex(key string, index int) ([]byte, error) {
	lst, unlock, err := l.getReadOnly(key)
	defer unlock()
	if err != nil || lst == nil {
		return nil, err
	}
	n := len(lst.items)
	if index < 0 {
		index = n + index
	}
	if index < 0 || index >= n {
		return nil, nil
	}
	return lst.items[index], nil
}

// LSet sets element at index.
func (l *ListStore) LSet(key string, index int, value []byte) error {
	lst, unlock, err := l.getOrCreate(key)
	defer unlock()
	if err != nil {
		return err
	}
	n := len(lst.items)
	if index < 0 {
		index = n + index
	}
	if index < 0 || index >= n {
		return util.Newf(util.ErrCodeOutOfRange, "index out of range")
	}
	lst.items[index] = value
	return nil
}

// LTrim trims the list to the specified range.
func (l *ListStore) LTrim(key string, start, stop int) error {
	lst, unlock, err := l.getOrCreate(key)
	defer unlock()
	if err != nil {
		return err
	}
	n := len(lst.items)
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
		lst.items = nil
	} else {
		lst.items = lst.items[start : stop+1]
	}
	return nil
}
