package persistence

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Compactor compacts old WAL segment files.
type Compactor struct {
	walDir string
}

// NewCompactor creates a compactor.
func NewCompactor(walDir string) *Compactor {
	return &Compactor{walDir: walDir}
}

// Compact removes archived WAL files (wal-*.log) older than current.
// Should be called after a snapshot is taken.
func (c *Compactor) Compact() (int, error) {
	entries, err := os.ReadDir(c.walDir)
	if err != nil {
		return 0, fmt.Errorf("compactor: readdir: %w", err)
	}
	removed := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "wal-") && strings.HasSuffix(e.Name(), ".log") {
			path := filepath.Join(c.walDir, e.Name())
			if err := os.Remove(path); err == nil {
				removed++
			}
		}
	}
	return removed, nil
}
