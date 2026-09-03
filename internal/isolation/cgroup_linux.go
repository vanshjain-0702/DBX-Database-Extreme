//go:build linux

package isolation

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ConfineCgroup moves pid into a tenant cgroup v2 with an optional memory.max.
// Failure is returned to the caller; ModeStrict treats this as best-effort
// because many containers do not delegate cgroup writes.
func ConfineCgroup(tenantID string, pid int, memoryMax int64) error {
	root, err := cgroupRoot()
	if err != nil {
		return err
	}
	dir := filepath.Join(root, "dbx-tenants", tenantID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("cgroup mkdir: %w", err)
	}
	if memoryMax > 0 {
		if err := os.WriteFile(filepath.Join(dir, "memory.max"), []byte(strconv.FormatInt(memoryMax, 10)), 0o644); err != nil {
			return fmt.Errorf("cgroup memory.max: %w", err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "cgroup.procs"), []byte(strconv.Itoa(pid)), 0o644); err != nil {
		return fmt.Errorf("cgroup.procs: %w", err)
	}
	return nil
}

func cgroupRoot() (string, error) {
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		// cgroup v2: 0::/path
		if !strings.HasPrefix(line, "0::") {
			continue
		}
		rel := strings.TrimPrefix(line, "0::")
		path := filepath.Join("/sys/fs/cgroup", rel)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		return "/sys/fs/cgroup", nil
	}
	return "", fmt.Errorf("cgroup v2 is not mounted")
}
