package auth

// ChannelPolicy enforces channel-level pub/sub access.
type ChannelPolicy struct {
	acl *ACLStore
}

// NewChannelPolicy creates a channel policy backed by an ACL store.
func NewChannelPolicy(acl *ACLStore) *ChannelPolicy {
	return &ChannelPolicy{acl: acl}
}

// CanSubscribe returns true if user can subscribe to channel.
func (c *ChannelPolicy) CanSubscribe(user *User, channel string) bool {
	if user.Permissions&PermPubSub == 0 && user.Permissions&PermRead == 0 {
		return false
	}
	return c.acl.CanAccessChannel(user, channel)
}

// CanPublish returns true if user can publish to channel.
func (c *ChannelPolicy) CanPublish(user *User, channel string) bool {
	if user.Permissions&PermWrite == 0 {
		return false
	}
	return c.acl.CanAccessChannel(user, channel)
}
