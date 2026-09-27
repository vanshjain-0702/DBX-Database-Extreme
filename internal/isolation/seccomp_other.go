//go:build !linux || !(amd64 || arm64)

package isolation

import "fmt"

// RestrictNetwork needs a seccomp filter written for this architecture.
func RestrictNetwork() error {
	return fmt.Errorf("isolation: the seccomp network seal supports linux/amd64 and linux/arm64 only")
}

// DisableDumping is paired with RestrictNetwork in LockDown.
func DisableDumping() error {
	return fmt.Errorf("isolation: non-dumpable workers are implemented for linux/amd64 and linux/arm64 only")
}
