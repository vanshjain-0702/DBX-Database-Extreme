package engine

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"hash/crc32"
	"io"
	"os"
)

var hnswMagic = []byte("DBXHNSW3")

type persistedHNSW struct {
	Version    uint32
	EntryPoint int
	MaxLayer   int
	Size       int
	EfSearch   int
	Nodes      map[int]*Node
}

// Save persists the HNSWGraph to a file using gob encoding.
func (g *HNSWGraph) Save(path string) error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var payload bytes.Buffer
	if err := gob.NewEncoder(&payload).Encode(persistedHNSW{
		Version: 3, EntryPoint: g.EntryPoint, MaxLayer: g.MaxLayer,
		Size: g.Size, EfSearch: g.EfSearch, Nodes: g.Nodes,
	}); err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	header := make([]byte, len(hnswMagic)+8)
	copy(header, hnswMagic)
	binary.BigEndian.PutUint32(header[len(hnswMagic):], uint32(payload.Len()))
	binary.BigEndian.PutUint32(header[len(hnswMagic)+4:], crc32.ChecksumIEEE(payload.Bytes()))
	if _, err := f.Write(header); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(payload.Bytes()); err != nil {
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

	_ = os.Remove(path)
	return os.Rename(tmpPath, path)
}

// LoadHNSWGraph loads an HNSWGraph from a file.
func LoadHNSWGraph(path string) (*HNSWGraph, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	header := make([]byte, len(hnswMagic)+8)
	if _, err := io.ReadFull(f, header); err != nil {
		return nil, err
	}
	if !bytes.Equal(header[:len(hnswMagic)], hnswMagic) {
		return nil, fmt.Errorf("unsupported HNSW cache format")
	}
	length := binary.BigEndian.Uint32(header[len(hnswMagic):])
	if length == 0 || length > 512<<20 {
		return nil, fmt.Errorf("invalid HNSW payload length %d", length)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(f, payload); err != nil {
		return nil, err
	}
	if crc32.ChecksumIEEE(payload) != binary.BigEndian.Uint32(header[len(hnswMagic)+4:]) {
		return nil, fmt.Errorf("HNSW checksum mismatch")
	}
	var persisted persistedHNSW
	if err := gob.NewDecoder(bytes.NewReader(payload)).Decode(&persisted); err != nil {
		return nil, err
	}
	if persisted.Version != 3 {
		return nil, fmt.Errorf("unsupported HNSW version %d", persisted.Version)
	}
	g := &HNSWGraph{
		Nodes: persisted.Nodes, EntryPoint: persisted.EntryPoint,
		MaxLayer: persisted.MaxLayer, Size: persisted.Size, EfSearch: persisted.EfSearch,
	}
	if g.Nodes == nil {
		g.Nodes = make(map[int]*Node)
	}
	if g.EfSearch < 1 {
		g.EfSearch = DefaultEfSearch
	}
	g.reindex()
	if err := g.Validate(persisted.Size); err != nil {
		return nil, err
	}
	return g, nil
}

// Validate rejects malformed graph caches before search can dereference them.
func (g *HNSWGraph) Validate(expectedSize int) error {
	if g.Size != expectedSize || len(g.Nodes) != expectedSize {
		return fmt.Errorf("HNSW size mismatch: graph=%d nodes=%d metadata=%d", g.Size, len(g.Nodes), expectedSize)
	}
	if expectedSize == 0 {
		if g.EntryPoint != -1 {
			return fmt.Errorf("empty HNSW has entry point %d", g.EntryPoint)
		}
		return nil
	}
	if _, ok := g.Nodes[g.EntryPoint]; !ok {
		return fmt.Errorf("HNSW entry point is missing")
	}
	for id, node := range g.Nodes {
		if node == nil || node.ID != id || node.Layer < 0 || node.Layer > 64 {
			return fmt.Errorf("invalid HNSW node %d", id)
		}
		for layer, edges := range node.Edges {
			if layer < 0 || layer > node.Layer {
				return fmt.Errorf("invalid HNSW layer %d for node %d", layer, id)
			}
			limit := M
			if layer == 0 {
				limit = M0
			}
			if len(edges) > limit*2 {
				return fmt.Errorf("too many HNSW edges for node %d layer %d", id, layer)
			}
			for _, neighbor := range edges {
				target, ok := g.Nodes[neighbor]
				if !ok || target == nil || target.Layer < layer || neighbor == id {
					return fmt.Errorf("invalid HNSW edge %d -> %d at layer %d", id, neighbor, layer)
				}
			}
		}
	}
	return nil
}

var hnswBundleMagic = []byte("DBXHNSW4")

type persistedHNSWBundle struct {
	Version uint32
	Shards  []persistedHNSW
}

func writeChecksummedHNSW(path string, magic []byte, payload []byte) error {
	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	header := make([]byte, len(magic)+8)
	copy(header, magic)
	binary.BigEndian.PutUint32(header[len(magic):], uint32(len(payload)))
	binary.BigEndian.PutUint32(header[len(magic)+4:], crc32.ChecksumIEEE(payload))
	if _, err := f.Write(header); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}
	if _, err := f.Write(payload); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	_ = os.Remove(path)
	return os.Rename(tmpPath, path)
}

func saveHNSWGraphs(path string, graphs []*HNSWGraph) error {
	shards := make([]persistedHNSW, len(graphs))
	for i, g := range graphs {
		if g == nil {
			continue
		}
		g.mu.RLock()
		shards[i] = persistedHNSW{
			Version: 3, EntryPoint: g.EntryPoint, MaxLayer: g.MaxLayer,
			Size: g.Size, EfSearch: g.EfSearch, Nodes: g.Nodes,
		}
		g.mu.RUnlock()
	}
	var payload bytes.Buffer
	if err := gob.NewEncoder(&payload).Encode(persistedHNSWBundle{Version: 4, Shards: shards}); err != nil {
		return err
	}
	return writeChecksummedHNSW(path, hnswBundleMagic, payload.Bytes())
}

func loadHNSWGraphs(path string) ([]*HNSWGraph, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	magic := make([]byte, len(hnswBundleMagic))
	if _, err := io.ReadFull(f, magic); err != nil {
		return nil, err
	}
	if !bytes.Equal(magic, hnswBundleMagic) {
		return nil, fmt.Errorf("unsupported HNSW cache format")
	}
	rest := make([]byte, 8)
	if _, err := io.ReadFull(f, rest); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(rest[:4])
	if length == 0 || length > 512<<20 {
		return nil, fmt.Errorf("invalid HNSW payload length %d", length)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(f, payload); err != nil {
		return nil, err
	}
	if crc32.ChecksumIEEE(payload) != binary.BigEndian.Uint32(rest[4:]) {
		return nil, fmt.Errorf("HNSW checksum mismatch")
	}
	var bundle persistedHNSWBundle
	if err := gob.NewDecoder(bytes.NewReader(payload)).Decode(&bundle); err != nil {
		return nil, err
	}
	if bundle.Version != 4 || len(bundle.Shards) != hnswShards {
		return nil, fmt.Errorf("unsupported HNSW bundle version %d shards %d", bundle.Version, len(bundle.Shards))
	}
	graphs := make([]*HNSWGraph, len(bundle.Shards))
	for i, shard := range bundle.Shards {
		g := &HNSWGraph{
			Nodes: shard.Nodes, EntryPoint: shard.EntryPoint,
			MaxLayer: shard.MaxLayer, Size: shard.Size, EfSearch: shard.EfSearch,
		}
		if g.Nodes == nil {
			g.Nodes = make(map[int]*Node)
		}
		if g.EfSearch < 1 {
			g.EfSearch = DefaultEfSearch
		}
		if g.Size == 0 {
			g.EntryPoint = -1
			g.MaxLayer = -1
		}
		g.reindex()
		graphs[i] = g
	}
	return graphs, nil
}
