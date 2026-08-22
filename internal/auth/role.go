package auth

// Role defines a named set of permissions.
type Role struct {
	Name        string
	Permissions int
	AllowedKeys []string
	AllowedChannels []string
}

var BuiltinRoles = map[string]*Role{
	"admin": {
		Name:            "admin",
		Permissions:     PermAll,
		AllowedKeys:     []string{"*"},
		AllowedChannels: []string{"*"},
	},
	"reader": {
		Name:            "reader",
		Permissions:     PermRead,
		AllowedKeys:     []string{"*"},
		AllowedChannels: []string{"*"},
	},
	"writer": {
		Name:            "writer",
		Permissions:     PermRead | PermWrite,
		AllowedKeys:     []string{"*"},
		AllowedChannels: []string{"*"},
	},
	"pubsub": {
		Name:            "pubsub",
		Permissions:     PermRead | PermPubSub,
		AllowedKeys:     []string{},
		AllowedChannels: []string{"*"},
	},
}

// ApplyRole applies a role's permissions to a user.
func ApplyRole(user *User, role *Role) {
	user.Permissions = role.Permissions
	user.AllowedKeys = role.AllowedKeys
	user.AllowedChannels = role.AllowedChannels
}
