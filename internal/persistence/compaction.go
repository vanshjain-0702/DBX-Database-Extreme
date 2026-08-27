package persistence

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	return c.CompactThrough(^uint64(0))
}

// CompactThrough removes only segments whose highest sequence is covered by
// an installed checkpoint.
func (c *Compactor) CompactThrough(checkpointSequence uint64) (int, error) {
	entries, err := os.ReadDir(c.walDir)
	if err != nil {
		return 0, fmt.Errorf("compactor: readdir: %w", err)
	}
	removed := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "wal-") && strings.HasSuffix(e.Name(), ".log") {
			seqText := strings.TrimSuffix(strings.TrimPrefix(e.Name(), "wal-"), ".log")
			segmentMax, parseErr := strconv.ParseUint(seqText, 10, 64)
			if parseErr != nil || segmentMax > checkpointSequence {
				continue
			}
			path := filepath.Join(c.walDir, e.Name())
			if err := os.Remove(path); err == nil {
				removed++
			}
		}
	}
	return removed, nil
}
