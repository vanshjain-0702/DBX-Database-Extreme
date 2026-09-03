//go:build !linux

package isolation

import (
	"fmt"
	"net"
)

// PeerPID is only implemented on Linux (SO_PEERCRED).
func PeerPID(conn net.Conn) (int, error) {
	return 0, fmt.Errorf("isolation: SO_PEERCRED is Linux-only")
}
