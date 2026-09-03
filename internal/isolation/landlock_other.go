//go:build !linux

package isolation

import "fmt"

// RestrictFilesystem is a Linux Landlock LSM operation.
func RestrictFilesystem(tenantDir string) error {
	return fmt.Errorf("isolation: Landlock is Linux-only")
}
