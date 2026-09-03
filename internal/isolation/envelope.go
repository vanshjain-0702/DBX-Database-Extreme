package isolation

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dbx/dbx/internal/security"
)

const (
	// WrappedDEKName is the on-disk wrapped tenant data-encryption key.
	WrappedDEKName = ".dbx-key.wrap"
	sealedMagic    = "DBXENC1\n"
	dekSize        = 32
	kekEnv         = "DBX_KEK"
)

var (
	errKEKMissing = errors.New("isolation: DBX_KEK must be 64 hex characters (256-bit wrapping key)")
	errBadWrap    = errors.New("isolation: wrapped tenant key is corrupt or was wrapped by a different KEK")
)

// LoadKEK reads the 256-bit wrapping key from DBX_KEK. The KEK never enters a
// tenant worker: the orchestrator unwraps the per-tenant DEK and hands only
// that DEK to the engine.
func LoadKEK() ([]byte, error) {
	raw := os.Getenv(kekEnv)
	if raw == "" {
		return nil, errKEKMissing
	}
	key, err := hex.DecodeString(raw)
	if err != nil || len(key) != dekSize {
		return nil, errKEKMissing
	}
	return key, nil
}

// GenerateDEK returns a fresh 256-bit tenant data-encryption key.
func GenerateDEK() ([]byte, error) {
	key := make([]byte, dekSize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

// WrapDEK encrypts a tenant DEK under the KEK and writes it into the tenant directory.
func WrapDEK(tenantDir string, kek, dek []byte) error {
	enc, err := security.NewEncryptor(kek)
	if err != nil {
		return err
	}
	wrapped, err := enc.Encrypt(dek)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(tenantDir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(tenantDir, WrappedDEKName)
	return os.WriteFile(path, wrapped, 0o600)
}

// UnwrapDEK loads the tenant DEK using the KEK. Missing wrap files are an error:
// engines that asked for encryption must not silently fall back to plaintext.
func UnwrapDEK(tenantDir string, kek []byte) ([]byte, error) {
	enc, err := security.NewEncryptor(kek)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(tenantDir, WrappedDEKName)
	wrapped, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("isolation: read wrapped DEK: %w", err)
	}
	dek, err := enc.Decrypt(wrapped)
	if err != nil || len(dek) != dekSize {
		return nil, errBadWrap
	}
	return dek, nil
}

// EnsureDEK returns the tenant DEK, creating and wrapping a new one if needed.
func EnsureDEK(tenantDir string, kek []byte) ([]byte, error) {
	path := filepath.Join(tenantDir, WrappedDEKName)
	if _, err := os.Stat(path); err == nil {
		return UnwrapDEK(tenantDir, kek)
	}
	dek, err := GenerateDEK()
	if err != nil {
		return nil, err
	}
	if err := WrapDEK(tenantDir, kek, dek); err != nil {
		return nil, err
	}
	return dek, nil
}

// ShredDEK removes the wrapped key so ciphertext in the tenant directory
// becomes unreadable. This is O(1) cryptographic deletion of data at rest.
// Running engines still hold the DEK in RAM until they are stopped.
func ShredDEK(tenantDir string) error {
	path := filepath.Join(tenantDir, WrappedDEKName)
	if err := overwriteAndRemove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func overwriteAndRemove(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	zeros := make([]byte, info.Size())
	if writeErr := os.WriteFile(path, zeros, 0o600); writeErr != nil {
		return writeErr
	}
	return os.Remove(path)
}

// Seal prepends a magic header to AES-256-GCM ciphertext. Unsealed plaintext
// files from earlier versions remain readable when OpenSealed is given an encryptor.
func Seal(enc *security.Encryptor, plaintext []byte) ([]byte, error) {
	if enc == nil {
		return append([]byte(nil), plaintext...), nil
	}
	ct, err := enc.Encrypt(plaintext)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(sealedMagic)+len(ct))
	out = append(out, sealedMagic...)
	return append(out, ct...), nil
}

// OpenSealed decrypts Seal output. Plaintext without the magic is returned as-is
// so existing tenant files can be migrated on first rewrite.
func OpenSealed(enc *security.Encryptor, data []byte) ([]byte, error) {
	if enc == nil {
		return append([]byte(nil), data...), nil
	}
	if len(data) >= len(sealedMagic) && string(data[:len(sealedMagic)]) == sealedMagic {
		return enc.Decrypt(data[len(sealedMagic):])
	}
	return append([]byte(nil), data...), nil
}

// WriteSealedFile writes a file that is ciphertext when enc is set.
func WriteSealedFile(path string, plaintext []byte, enc *security.Encryptor) error {
	payload, err := Seal(enc, plaintext)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return err
	}
	f, err := os.OpenFile(tmp, os.O_RDWR, 0o600)
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	syncErr := f.Sync()
	_ = f.Close()
	if syncErr != nil {
		_ = os.Remove(tmp)
		return syncErr
	}
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}

// ReadSealedFile reads WriteSealedFile output.
func ReadSealedFile(path string, enc *security.Encryptor) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return OpenSealed(enc, data)
}

// NewEncryptor is a thin wrapper so callers do not import security for the
// common 32-byte key case.
func NewEncryptor(dek []byte) (*security.Encryptor, error) {
	return security.NewEncryptor(dek)
}
