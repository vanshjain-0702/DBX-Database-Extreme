package persistence

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// BackupStore handles backup creation and restoration.
type BackupStore struct {
	snapshotDir string
	backupDir   string
}

// NewBackupStore creates a backup store.
func NewBackupStore(snapshotDir, backupDir string) *BackupStore {
	os.MkdirAll(backupDir, 0755)
	return &BackupStore{snapshotDir: snapshotDir, backupDir: backupDir}
}

// CreateBackup copies the latest snapshot to the backup dir.
func (b *BackupStore) CreateBackup() (string, error) {
	snap := (&Snapshotter{dir: b.snapshotDir}).Latest()
	if snap == "" {
		return "", fmt.Errorf("backup: no snapshot found")
	}
	dst := filepath.Join(b.backupDir, fmt.Sprintf("backup-%d.rdb", time.Now().UnixNano()))
	if err := copyFile(snap, dst); err != nil {
		return "", fmt.Errorf("backup: copy: %w", err)
	}
	return dst, nil
}

// ListBackups returns all backup file paths.
func (b *BackupStore) ListBackups() ([]string, error) {
	entries, err := os.ReadDir(b.backupDir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if !e.IsDir() {
			paths = append(paths, filepath.Join(b.backupDir, e.Name()))
		}
	}
	return paths, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
