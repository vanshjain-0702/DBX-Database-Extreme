package persistence

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/dbx/dbx/internal/security"
)

func TestEncryptedWALRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	enc, err := security.NewEncryptor(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "wal")
	wal, err := OpenWALEncrypted(dir, "always", 64, enc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wal.WriteTransaction([]WALEffect{{
		Type: RecordSet, Key: "session", Value: []byte("secret-token"),
	}}); err != nil {
		t.Fatal(err)
	}
	if err := wal.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "wal.log"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("secret-token")) {
		t.Fatal("WAL stored plaintext")
	}
	reopened, err := OpenWALEncrypted(dir, "always", 64, enc)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	records, err := reopened.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || string(records[0].Effects[0].Value) != "secret-token" {
		t.Fatalf("records=%+v", records)
	}
}
