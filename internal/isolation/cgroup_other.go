//go:build !linux

package isolation

import "fmt"

// ConfineCgroup is a Linux cgroups v2 operation.
func ConfineCgroup(tenantID string, pid int, memoryMax int64) error {
	return fmt.Errorf("isolation: cgroups v2 are Linux-only")
}
