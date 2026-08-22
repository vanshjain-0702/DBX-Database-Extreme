package engine

import (
	"fmt"
	"strings"
	"time"

	"github.com/dbx/dbx/internal/protocol"
	"github.com/dbx/dbx/internal/util"
)

// StreamEntry represents a single stream entry.
type StreamEntry struct {
	ID     string
	Fields map[string]string
}

// dbxStream stores ordered stream entries.
type dbxStream struct {
	entries  []StreamEntry
	lastID   string
	groups   map[string]*consumerGroup
}

type consumerGroup struct {
	Name       string
	LastID     string
	Consumers  map[string]*consumer
	Pending    []StreamEntry
}

type consumer struct {
	Name    string
	Pending []StreamEntry
}

// StreamStore provides stream operations.
type StreamStore struct{ kv *KVStore }

func NewStreamStore(kv *KVStore) *StreamStore { return &StreamStore{kv: kv} }

func (s *StreamStore) getOrCreate(key string) (*dbxStream, func(), error) {
	e, unlock := s.kv.GetForWrite(key)
	if e == nil {
		st := &dbxStream{groups: make(map[string]*consumerGroup)}
		sh := s.kv.shard(key)
		sh.data[key] = &Entry{Value: st, Type: protocol.TypeStream}
		return st, unlock, nil
	}
	if e.Type != protocol.TypeStream {
		unlock()
		return nil, func() {}, util.ErrWrongType
	}
	return e.Value.(*dbxStream), unlock, nil
}

func (s *StreamStore) getReadOnly(key string) (*dbxStream, func(), error) {
	e, unlock := s.kv.GetForRead(key)
	if e == nil {
		return nil, func(){}, nil
	}
	if e.Type != protocol.TypeStream {
		unlock()
		return nil, func(){}, util.ErrWrongType
	}
	return e.Value.(*dbxStream), unlock, nil
}

// generateID generates a stream ID in the format <ms>-<seq>.
func generateID(last string) string {
	ms := time.Now().UnixMilli()
	if last != "" {
		var lms int64
		var lseq int64
		fmt.Sscanf(last, "%d-%d", &lms, &lseq)
		if ms == lms {
			return fmt.Sprintf("%d-%d", ms, lseq+1)
		}
	}
	return fmt.Sprintf("%d-0", ms)
}

// XAdd appends an entry. Returns the entry ID.
func (s *StreamStore) XAdd(key, id string, fields map[string]string, maxLen int) (string, error) {
	st, unlock, err := s.getOrCreate(key)
	defer unlock()
	if err != nil {
		return "", err
	}
	if id == "" || id == "*" {
		id = generateID(st.lastID)
	}
	entry := StreamEntry{ID: id, Fields: fields}
	st.entries = append(st.entries, entry)
	st.lastID = id
	if maxLen > 0 && len(st.entries) > maxLen {
		st.entries = st.entries[len(st.entries)-maxLen:]
	}
	return id, nil
}

// XRange returns entries between start and end ID (inclusive).
func (s *StreamStore) XRange(key, start, end string, count int) ([]StreamEntry, error) {
	st, unlock, err := s.getReadOnly(key)
	defer unlock()
	if err != nil || st == nil {
		return nil, err
	}
	var result []StreamEntry
	for _, e := range st.entries {
		if (start == "-" || e.ID >= start) && (end == "+" || e.ID <= end) {
			result = append(result, e)
			if count > 0 && len(result) >= count {
				break
			}
		}
	}
	return result, nil
}

// XRead reads entries from one or more streams starting after the given IDs.
func (s *StreamStore) XRead(keys, ids []string, count int) (map[string][]StreamEntry, error) {
	result := make(map[string][]StreamEntry)
	for i, key := range keys {
		startID := ids[i]
		entries, err := s.XRange(key, startID, "+", count)
		if err != nil {
			return nil, err
		}
		// Filter to entries strictly after startID
		var filtered []StreamEntry
		for _, e := range entries {
			if e.ID > startID {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) > 0 {
			result[key] = filtered
		}
	}
	return result, nil
}

// XLen returns the number of entries in the stream.
func (s *StreamStore) XLen(key string) (int, error) {
	st, unlock, err := s.getReadOnly(key)
	defer unlock()
	if err != nil || st == nil {
		return 0, err
	}
	return len(st.entries), nil
}

// XGroupCreate creates a consumer group on the stream.
func (s *StreamStore) XGroupCreate(key, group, startID string) error {
	st, unlock, err := s.getOrCreate(key)
	defer unlock()
	if err != nil {
		return err
	}
	if _, exists := st.groups[group]; exists {
		return fmt.Errorf("BUSYGROUP consumer group '%s' already exists", group)
	}
	st.groups[group] = &consumerGroup{
		Name:      group,
		LastID:    startID,
		Consumers: make(map[string]*consumer),
	}
	return nil
}

// XReadGroup reads entries for a consumer group.
func (s *StreamStore) XReadGroup(key, group, consumerName, lastID string, count int) ([]StreamEntry, error) {
	st, unlock, err := s.getOrCreate(key)
	defer unlock()
	if err != nil {
		return nil, err
	}
	g, ok := st.groups[group]
	if !ok {
		return nil, fmt.Errorf("NOGROUP no such consumer group '%s'", group)
	}
	if _, ok := g.Consumers[consumerName]; !ok {
		g.Consumers[consumerName] = &consumer{Name: consumerName}
	}
	c := g.Consumers[consumerName]
	var result []StreamEntry
	for _, e := range st.entries {
		if e.ID > g.LastID {
			result = append(result, e)
			c.Pending = append(c.Pending, e)
			g.LastID = e.ID
			if count > 0 && len(result) >= count {
				break
			}
		}
	}
	return result, nil
}

// XAck acknowledges entries in a consumer group.
func (s *StreamStore) XAck(key, group string, ids []string) (int, error) {
	st, unlock, err := s.getOrCreate(key)
	defer unlock()
	if err != nil {
		return 0, err
	}
	g, ok := st.groups[group]
	if !ok {
		return 0, nil
	}
	acked := 0
	idSet := make(map[string]struct{})
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	var remaining []StreamEntry
	for _, e := range g.Pending {
		if _, del := idSet[e.ID]; del {
			acked++
		} else {
			remaining = append(remaining, e)
		}
	}
	g.Pending = remaining
	for _, c := range g.Consumers {
		var cRemaining []StreamEntry
		for _, e := range c.Pending {
			if _, del := idSet[e.ID]; !del {
				cRemaining = append(cRemaining, e)
			}
		}
		c.Pending = cRemaining
	}
	return acked, nil
}

// ParseStreamFields parses alternating field-value pairs.
func ParseStreamFields(args [][]byte) (map[string]string, error) {
	if len(args)%2 != 0 {
		return nil, fmt.Errorf("odd number of field-value pairs")
	}
	fields := make(map[string]string, len(args)/2)
	for i := 0; i < len(args); i += 2 {
		fields[string(args[i])] = string(args[i+1])
	}
	return fields, nil
}

// ParseXAddArgs parses XADD key [MAXLEN count] id field value ...
func ParseXAddArgs(args [][]byte) (key, id string, fields map[string]string, maxLen int, err error) {
	if len(args) < 4 {
		err = util.ErrSyntax
		return
	}
	key = string(args[0])
	idx := 1
	if strings.ToUpper(string(args[idx])) == "MAXLEN" {
		idx++
		if idx >= len(args) {
			err = util.ErrSyntax
			return
		}
		// skip ~ if present
		if string(args[idx]) == "~" {
			idx++
		}
		fmt.Sscanf(string(args[idx]), "%d", &maxLen)
		idx++
	}
	if idx >= len(args) {
		err = util.ErrSyntax
		return
	}
	id = string(args[idx])
	idx++
	fields, err = ParseStreamFields(args[idx:])
	return
}
