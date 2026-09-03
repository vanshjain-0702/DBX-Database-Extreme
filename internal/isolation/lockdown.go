package isolation

import (
	"fmt"
	"os"
)

// LockDown applies the kernel seals that a worker can apply to itself after
// listeners are bound: Landlock, then (optionally) a reminder that cgroups are
// attached by the orchestrator. The KEK must already be absent from the
// environment.
func LockDown(tenantDir string) error {
	if os.Getenv("DBX_KEK") != "" {
		return fmt.Errorf("isolation: tenant worker must not receive DBX_KEK")
	}
	if err := RestrictFilesystem(tenantDir); err != nil {
		return err
	}
	return nil
}
