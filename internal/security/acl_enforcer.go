// Package security provides ACL enforcement, rate limiting, encryption, tenant quotas, and audit.
package security

import (
	"fmt"

	"github.com/dbx/dbx/internal/auth"
	"github.com/dbx/dbx/internal/protocol"
)

// ACLEnforcer enforces ACL rules on incoming commands.
type ACLEnforcer struct {
	acl *auth.ACLStore
}

// NewACLEnforcer creates a new enforcer.
func NewACLEnforcer(acl *auth.ACLStore) *ACLEnforcer {
	return &ACLEnforcer{acl: acl}
}

// Enforce checks if user can execute cmd on key. Returns nil if allowed.
func (e *ACLEnforcer) Enforce(user *auth.User, cmd *protocol.Command) error {
	if user == nil {
		return fmt.Errorf("NOAUTH authentication required")
	}
	if !e.acl.CanExecute(user, cmd.Normalized()) {
		return fmt.Errorf("NOPERM this user has no permissions to run the '%s' command", cmd.Name)
	}
	// Check key permissions
	info, ok := protocol.Lookup(cmd.Normalized())
	if ok && info.KeyIndex > 0 && info.KeyIndex <= len(cmd.Args) {
		key := cmd.Arg(info.KeyIndex - 1)
		if key != "" && !e.acl.CanAccessKey(user, key) {
			return fmt.Errorf("NOPERM no permissions to access key '%s'", key)
		}
	}
	return nil
}
