package persistence

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dbx/dbx/internal/isolation"
)

type BackupManifest struct {
	Version            int          `json:"version"`
	TenantID           string       `json:"tenant_id"`
	Format             string       `json:"format"`
	CheckpointSequence uint64       `json:"checkpoint_sequence"`
	CreatedAt          time.Time    `json:"created_at"`
	Files              []BackupFile `json:"files"`
}

type BackupFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// CreateBackupArchive writes a checksummed checkpoint archive. Callers must
// hold the tenant maintenance lock for the duration.
func CreateBackupArchive(tenantID, dataDir, snapshotPath, outputPath string, sequence uint64) (BackupManifest, error) {
	var candidates []string
	candidates = append(candidates, snapshotPath)
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return BackupManifest{}, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".vec") || strings.HasSuffix(name, ".vec.meta") ||
			strings.HasSuffix(name, ".vec.hnsw") {
			candidates = append(candidates, filepath.Join(dataDir, name))
		}
		// The wrapped tenant DEK travels with the archive, otherwise restoring
		// into a purged directory produces ciphertext nobody can open. It is
		// sealed under the operator KEK, so the archive stays useless without it.
		if name == isolation.WrappedDEKName {
			candidates = append(candidates, filepath.Join(dataDir, name))
		}
	}
	manifest := BackupManifest{
		Version: 2, TenantID: tenantID, Format: "dbx-v1",
		CheckpointSequence: sequence, CreatedAt: time.Now().UTC(),
	}
	for _, path := range candidates {
		relative, err := filepath.Rel(dataDir, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return BackupManifest{}, fmt.Errorf("backup file is outside tenant data directory: %s", path)
		}
		file, err := os.Open(path)
		if err != nil {
			return BackupManifest{}, err
		}
		hash := sha256.New()
		size, copyErr := io.Copy(hash, file)
		file.Close()
		if copyErr != nil {
			return BackupManifest{}, copyErr
		}
		manifest.Files = append(manifest.Files, BackupFile{
			Path: filepath.ToSlash(relative), Size: size, SHA256: hex.EncodeToString(hash.Sum(nil)),
		})
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0700); err != nil {
		return BackupManifest{}, err
	}
	tmp := outputPath + ".tmp"
	output, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return BackupManifest{}, err
	}
	archive := zip.NewWriter(output)
	manifestWriter, err := archive.Create("manifest.json")
	if err == nil {
		err = json.NewEncoder(manifestWriter).Encode(manifest)
	}
	for _, item := range manifest.Files {
		if err != nil {
			break
		}
		writer, createErr := archive.CreateHeader(&zip.FileHeader{Name: item.Path, Method: zip.Deflate})
		if createErr != nil {
			err = createErr
			break
		}
		source, openErr := os.Open(filepath.Join(dataDir, filepath.FromSlash(item.Path)))
		if openErr != nil {
			err = openErr
			break
		}
		_, err = io.Copy(writer, source)
		source.Close()
	}
	if closeErr := archive.Close(); err == nil {
		err = closeErr
	}
	if syncErr := output.Sync(); err == nil {
		err = syncErr
	}
	if closeErr := output.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return BackupManifest{}, err
	}
	_ = os.Remove(outputPath)
	if err := os.Rename(tmp, outputPath); err != nil {
		return BackupManifest{}, err
	}
	return manifest, nil
}

// ExtractAndValidateBackup extracts only manifest-listed files, enforcing
// path, size, format, and checksum limits.
func ExtractAndValidateBackup(archivePath, stagingDir, tenantID string, maxBytes int64) (BackupManifest, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return BackupManifest{}, err
	}
	defer reader.Close()
	var manifest BackupManifest
	files := make(map[string]*zip.File)
	for _, file := range reader.File {
		if file.Name == "manifest.json" {
			if file.UncompressedSize64 > 1<<20 {
				return manifest, fmt.Errorf("backup manifest exceeds limit")
			}
			stream, err := file.Open()
			if err != nil {
				return manifest, err
			}
			err = json.NewDecoder(stream).Decode(&manifest)
			stream.Close()
			if err != nil {
				return manifest, err
			}
			continue
		}
		files[file.Name] = file
	}
	if manifest.Version != 2 || manifest.Format != "dbx-v1" || manifest.TenantID != tenantID {
		return manifest, fmt.Errorf("backup manifest is incompatible with tenant %q", tenantID)
	}
	var total int64
	for _, item := range manifest.Files {
		clean := filepath.Clean(filepath.FromSlash(item.Path))
		if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return manifest, fmt.Errorf("unsafe backup path %q", item.Path)
		}
		total += item.Size
		if item.Size < 0 || (maxBytes > 0 && total > maxBytes) {
			return manifest, fmt.Errorf("backup exceeds restore size limit")
		}
		zipped := files[item.Path]
		if zipped == nil || int64(zipped.UncompressedSize64) != item.Size {
			return manifest, fmt.Errorf("backup file missing or size mismatch: %s", item.Path)
		}
		target := filepath.Join(stagingDir, clean)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return manifest, err
		}
		input, err := zipped.Open()
		if err != nil {
			return manifest, err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			input.Close()
			return manifest, err
		}
		hash := sha256.New()
		written, copyErr := io.Copy(io.MultiWriter(output, hash), io.LimitReader(input, item.Size+1))
		input.Close()
		if copyErr == nil && written != item.Size {
			copyErr = fmt.Errorf("backup file length mismatch: %s", item.Path)
		}
		if copyErr == nil && hex.EncodeToString(hash.Sum(nil)) != item.SHA256 {
			copyErr = fmt.Errorf("backup checksum mismatch: %s", item.Path)
		}
		if syncErr := output.Sync(); copyErr == nil {
			copyErr = syncErr
		}
		output.Close()
		if copyErr != nil {
			return manifest, copyErr
		}
	}
	return manifest, nil
}
