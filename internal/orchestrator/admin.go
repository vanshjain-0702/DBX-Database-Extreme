package orchestrator

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type APIKey struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Prefix    string    `json:"prefix"`
	Hash      string    `json:"hash"`
	CreatedAt time.Time `json:"created_at"`
}

// AdminCredentials holds the hashed password and API keys.
type AdminCredentials struct {
	Username     string   `json:"username"`
	PasswordHash string   `json:"password_hash"`
	APIKeys      []APIKey `json:"api_keys"`
}

// AdminStore manages the persistent admin credentials.
type AdminStore struct {
	mu       sync.RWMutex
	filePath string
	creds    AdminCredentials
}

// NewAdminStore creates or loads an AdminStore. A bootstrap password is
// required only when the credential file does not yet exist.
func NewAdminStore(filePath, bootstrapPassword string) (*AdminStore, error) {
	store := &AdminStore{
		filePath: filePath,
	}

	if err := store.load(); err != nil {
		if os.IsNotExist(err) {
			if len(bootstrapPassword) < 12 {
				return nil, fmt.Errorf("DBX_ADMIN_PASSWORD must be at least 12 characters for first-run setup")
			}
			err = store.UpdatePassword("admin", bootstrapPassword)
			if err != nil {
				return nil, fmt.Errorf("failed to create default admin: %w", err)
			}
		} else {
			return nil, err
		}
	}
	return store, nil
}

func (s *AdminStore) load() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.creds)
}

func (s *AdminStore) save() error {
	data, err := json.MarshalIndent(s.creds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0600) // Restricted permissions
}

// VerifyPassword checks if the provided password is correct.
func (s *AdminStore) VerifyPassword(username, password string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.creds.Username != username {
		return false
	}

	err := bcrypt.CompareHashAndPassword([]byte(s.creds.PasswordHash), []byte(password))
	return err == nil
}

// UpdatePassword sets a new password.
func (s *AdminStore) UpdatePassword(username, newPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	s.creds.Username = username
	s.creds.PasswordHash = string(hash)

	return s.save()
}

func generateRandomString(n int) (string, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// GenerateAPIKey creates a new API key, stores the hash, and returns the plaintext key.
func (s *AdminStore) GenerateAPIKey(name string) (string, *APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rawSecret, err := generateRandomString(32)
	if err != nil {
		return "", nil, err
	}
	
	id, err := generateRandomString(8)
	if err != nil {
		return "", nil, err
	}

	prefix := "dbx_" + rawSecret[:8]
	fullKey := "dbx_" + rawSecret
	
	hash, err := bcrypt.GenerateFromPassword([]byte(fullKey), bcrypt.DefaultCost)
	if err != nil {
		return "", nil, err
	}

	key := APIKey{
		ID:        id,
		Name:      name,
		Prefix:    prefix,
		Hash:      string(hash),
		CreatedAt: time.Now(),
	}

	if s.creds.APIKeys == nil {
		s.creds.APIKeys = make([]APIKey, 0)
	}
	s.creds.APIKeys = append(s.creds.APIKeys, key)

	if err := s.save(); err != nil {
		return "", nil, err
	}

	return fullKey, &key, nil
}

// ListAPIKeys returns a copy of the API keys (without hashes).
func (s *AdminStore) ListAPIKeys() []APIKey {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	keys := make([]APIKey, 0) // ensure we never return nil
	for _, k := range s.creds.APIKeys {
		keys = append(keys, APIKey{
			ID:        k.ID,
			Name:      k.Name,
			Prefix:    k.Prefix,
			CreatedAt: k.CreatedAt,
		})
	}
	return keys
}

// RevokeAPIKey removes an API key by ID.
func (s *AdminStore) RevokeAPIKey(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	found := false
	var newKeys []APIKey
	for _, k := range s.creds.APIKeys {
		if k.ID == id {
			found = true
			continue
		}
		newKeys = append(newKeys, k)
	}

	if !found {
		return fmt.Errorf("api key not found")
	}

	s.creds.APIKeys = newKeys
	return s.save()
}
