// Package auth provides ACL-based access control.
package auth

import (
	"strings"
	"sync"
)

// Permission bitmask constants.
const (
	PermRead   = 1 << 0
	PermWrite  = 1 << 1
	PermAdmin  = 1 << 2
	PermPubSub = 1 << 3
	PermAll    = PermRead | PermWrite | PermAdmin | PermPubSub
)

// User represents an ACL user.
type User struct {
	Name            string
	PasswordHash    string
	Enabled         bool
	Permissions     int
	AllowedKeys     []string // glob patterns
	AllowedChannels []string
	NoPass          bool // if true, any password works
}

// ACLStore manages users and their permissions.
type ACLStore struct {
	mu    sync.RWMutex
	users map[string]*User
}

// NewACLStore creates an ACL store with a default user.
func NewACLStore() *ACLStore {
	store := &ACLStore{users: make(map[string]*User)}
	// Default user has all permissions and no password
	store.users["default"] = &User{
		Name:            "default",
		Enabled:         true,
		Permissions:     PermAll,
		NoPass:          true,
		AllowedKeys:     []string{"*"},
		AllowedChannels: []string{"*"},
	}
	return store
}

// DisableDefault removes the development NoPass identity.
func (a *ACLStore) DisableDefault() {
	a.mu.Lock()
	delete(a.users, "default")
	a.mu.Unlock()
}

// SetDefaultPassword replaces the built-in development user with a password
// protected user. It is used by the server when require_password is enabled.
func (a *ACLStore) SetDefaultPassword(name, password string) {
	a.AddUser(CreateUser(name, password, PermAll))
}

// GetUser returns a user by name.
func (a *ACLStore) GetUser(name string) *User {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.users[name]
}

// AddUser adds or updates a user.
func (a *ACLStore) AddUser(u *User) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.users[u.Name] = u
}

// DeleteUser removes a user.
func (a *ACLStore) DeleteUser(name string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.users[name]; ok {
		delete(a.users, name)
		return true
	}
	return false
}

// ListUsers returns all user names.
func (a *ACLStore) ListUsers() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	names := make([]string, 0, len(a.users))
	for name := range a.users {
		names = append(names, name)
	}
	return names
}

// CanExecute returns true if user can execute cmdName.
func (a *ACLStore) CanExecute(user *User, cmdName string) bool {
	if !user.Enabled {
		return false
	}
	cmdName = strings.ToUpper(cmdName)
	// Check admin commands
	if isAdminCommand(cmdName) {
		return user.Permissions&PermAdmin != 0
	}
	if isWriteCommand(cmdName) {
		return user.Permissions&PermWrite != 0
	}
	return user.Permissions&PermRead != 0
}

// CanAccessKey returns true if user can access the given key.
func (a *ACLStore) CanAccessKey(user *User, key string) bool {
	if !user.Enabled {
		return false
	}
	for _, pattern := range user.AllowedKeys {
		if pattern == "*" || matchKeyPattern(pattern, key) {
			return true
		}
	}
	return false
}

// CanAccessChannel returns true if user can access a pub/sub channel.
func (a *ACLStore) CanAccessChannel(user *User, channel string) bool {
	for _, pattern := range user.AllowedChannels {
		if pattern == "*" || matchKeyPattern(pattern, channel) {
			return true
		}
	}
	return false
}

func matchKeyPattern(pattern, key string) bool {
	// Simple glob matching
	if pattern == "*" {
		return true
	}
	return strings.HasPrefix(key, strings.TrimSuffix(pattern, "*"))
}

var adminCommands = map[string]bool{
	"CONFIG": true, "DEBUG": true, "FLUSHALL": true, "FLUSHDB": true,
	"SAVE": true, "BGSAVE": true, "CLUSTER": true, "ACL": true,
	"REPLICAOF": true, "SLAVEOF": true, "SHUTDOWN": true,
	"VCOMPACT": true,
}

var writeCommands = map[string]bool{
	"SET": true, "SETEX": true, "DEL": true, "EXPIRE": true, "RENAME": true, "INCR": true,
	"DECR": true, "INCRBY": true, "DECRBY": true, "APPEND": true, "SETRANGE": true,
	"MSET": true, "SETNX": true, "GETSET": true, "HSET": true, "HDEL": true,
	"HMSET": true, "HINCRBY": true, "LPUSH": true, "RPUSH": true, "LPOP": true,
	"RPOP": true, "LSET": true, "LREM": true, "LTRIM": true, "SADD": true,
	"SREM": true, "SPOP": true, "ZADD": true, "ZREM": true, "ZINCRBY": true,
	"XADD": true, "XGROUP": true, "XACK": true, "SETBIT": true,
	"GEOADD": true, "PERSIST": true, "MULTI": true, "EXEC": true, "DISCARD": true,
	"PUBLISH": true,
	"VADD":    true, "VADD_BATCH": true, "VADDBIN": true, "VDEL": true,
}

func isAdminCommand(cmd string) bool { return adminCommands[cmd] }
func isWriteCommand(cmd string) bool { return writeCommands[cmd] }
