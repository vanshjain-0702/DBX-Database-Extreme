package engine

import (
	"encoding/json"
	"fmt"

	"github.com/dbx/dbx/internal/protocol"
	"github.com/dbx/dbx/internal/util"
)

// DocumentStore provides JSON document storage.
type DocumentStore struct{ kv *KVStore }

func NewDocumentStore(kv *KVStore) *DocumentStore { return &DocumentStore{kv: kv} }

// Set stores a JSON document at key.
func (d *DocumentStore) Set(key string, doc interface{}) error {
	data, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("document marshal: %w", err)
	}
	d.kv.Set(key, data, protocol.TypeDocument, 0)
	return nil
}

// Get retrieves a JSON document.
func (d *DocumentStore) Get(key string) (json.RawMessage, error) {
	e := d.kv.Get(key)
	if e == nil {
		return nil, nil
	}
	if e.Type != protocol.TypeDocument {
		return nil, util.ErrWrongType
	}
	return json.RawMessage(e.Value.([]byte)), nil
}

// GetPath retrieves a value at a JSON path (dot-notation).
func (d *DocumentStore) GetPath(key, path string) (interface{}, error) {
	raw, err := d.Get(key)
	if err != nil || raw == nil {
		return nil, err
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("document unmarshal: %w", err)
	}
	return getJSONPath(doc, path), nil
}

// SetPath sets a value at a JSON path.
func (d *DocumentStore) SetPath(key, path string, value interface{}) error {
	raw, err := d.Get(key)
	if err != nil {
		return err
	}
	var doc map[string]interface{}
	if raw != nil {
		if err := json.Unmarshal(raw, &doc); err != nil {
			return err
		}
	} else {
		doc = make(map[string]interface{})
	}
	setJSONPath(doc, path, value)
	return d.Set(key, doc)
}

func getJSONPath(doc map[string]interface{}, path string) interface{} {
	if path == "." || path == "" {
		return doc
	}
	if path[0] == '.' {
		path = path[1:]
	}
	parts := splitPath(path)
	var current interface{} = doc
	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		current = m[part]
	}
	return current
}

func setJSONPath(doc map[string]interface{}, path string, value interface{}) {
	if path == "." || path == "" {
		return
	}
	if path[0] == '.' {
		path = path[1:]
	}
	parts := splitPath(path)
	current := doc
	for i, part := range parts[:len(parts)-1] {
		next, ok := current[part].(map[string]interface{})
		if !ok {
			next = make(map[string]interface{})
			current[parts[i]] = next
		}
		current = next
	}
	current[parts[len(parts)-1]] = value
}

func splitPath(path string) []string {
	var parts []string
	var cur string
	for _, c := range path {
		if c == '.' {
			if cur != "" {
				parts = append(parts, cur)
				cur = ""
			}
		} else {
			cur += string(c)
		}
	}
	if cur != "" {
		parts = append(parts, cur)
	}
	return parts
}
