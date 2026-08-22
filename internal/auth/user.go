package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// HashPassword returns a SHA-256 hex hash of password.
func HashPassword(password string) string {
	h := sha256.Sum256([]byte(password))
	return hex.EncodeToString(h[:])
}

// CreateUser creates a new user with the given password.
func CreateUser(name, password string, perms int) *User {
	u := &User{
		Name:            name,
		Enabled:         true,
		Permissions:     perms,
		AllowedKeys:     []string{"*"},
		AllowedChannels: []string{"*"},
	}
	if password == "" {
		u.NoPass = true
	} else {
		u.PasswordHash = HashPassword(password)
	}
	return u
}

// Authenticate checks if the given password matches the user.
func Authenticate(user *User, password string) error {
	if !user.Enabled {
		return fmt.Errorf("NOAUTH user is disabled")
	}
	if user.NoPass {
		return nil
	}
	if HashPassword(password) != user.PasswordHash {
		return fmt.Errorf("WRONGPASS invalid username-password pair")
	}
	return nil
}
