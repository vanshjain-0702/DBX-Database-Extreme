package isolation

import (
	"fmt"
	"os"
)

// LockDown applies the kernel seals that a worker can apply to itself after
// listeners are bound: non-dumpable memory, Landlock on the filesystem (and
// TCP where the kernel supports it), then a seccomp filter that allows only
// Unix sockets. cgroups are attached by the orchestrator. The KEK must already
// be absent from the environment. Every step fails closed.
func LockDown(tenantDir string) error {
	if os.Getenv("DBX_KEK") != "" {
		return fmt.Errorf("isolation: tenant worker must not receive DBX_KEK")
	}
	if err := DisableDumping(); err != nil {
		return err
	}
	if err := RestrictFilesystem(tenantDir); err != nil {
		return err
	}
	if err := RestrictNetwork(); err != nil {
		return err
	}
	return nil
}
