// Package query provides the query router.
package query

import (
	"github.com/dbx/dbx/internal/cluster"
	"github.com/dbx/dbx/internal/util"
)

// Router routes commands to the correct cluster node.
type Router struct {
	ring   *cluster.Ring
	selfID string
}

// NewRouter creates a query router.
func NewRouter(ring *cluster.Ring, selfID string) *Router {
	return &Router{ring: ring, selfID: selfID}
}

// RouteKey returns redirect info if the key belongs to another node.
// Returns ("", false) if the key belongs to this node.
func (r *Router) RouteKey(key string) (redirectAddr string, isRedirect bool) {
	if r.ring == nil {
		return "", false
	}
	node := r.ring.NodeForKey(key)
	if node == nil || node.ID == r.selfID {
		return "", false
	}
	slot := util.HashSlot(key)
	return cluster.Redirect(int(slot), node), true
}
