package engine

import (
	"encoding/gob"
	"os"
)

// Save persists the HNSWGraph to a file using gob encoding.
func (g *HNSWGraph) Save(path string) error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	encoder := gob.NewEncoder(f)
	if err := encoder.Encode(g.EntryPoint); err != nil {
		f.Close()
		return err
	}
	if err := encoder.Encode(g.MaxLayer); err != nil {
		f.Close()
		return err
	}
	if err := encoder.Encode(g.Size); err != nil {
		f.Close()
		return err
	}
	if err := encoder.Encode(g.Nodes); err != nil {
		f.Close()
		return err
	}

	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

// LoadHNSWGraph loads an HNSWGraph from a file.
func LoadHNSWGraph(path string) (*HNSWGraph, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	g := NewHNSWGraph()

	decoder := gob.NewDecoder(f)
	if err := decoder.Decode(&g.EntryPoint); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&g.MaxLayer); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&g.Size); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&g.Nodes); err != nil {
		return nil, err
	}

	return g, nil
}
