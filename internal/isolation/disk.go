package isolation

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DataDirLooksEncrypted reports whether dir appears to live on LUKS, fscrypt,
// or another encrypting mount. This is an operator check, not a guarantee:
// a bind-mount over an encrypted volume can still look like plain ext4.
func DataDirLooksEncrypted(dir string) (bool, string) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return false, err.Error()
	}
	if runtime.GOOS != "linux" {
		return false, "disk-encryption probe is Linux-only"
	}
	return linuxMountEncrypted(abs)
}

func linuxMountEncrypted(abs string) (bool, string) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false, err.Error()
	}
	bestDev, bestMP, bestFS, bestOpts := "", "/", "", ""
	bestLen := 0
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		mp := unescapeMount(fields[1])
		if !strings.HasPrefix(abs, mp) {
			continue
		}
		if len(mp) < bestLen {
			continue
		}
		bestLen = len(mp)
		bestDev, bestMP, bestFS, bestOpts = fields[0], mp, fields[2], fields[3]
	}
	note := bestFS + " on " + bestMP
	switch {
	case strings.HasPrefix(bestDev, "/dev/mapper/"),
		strings.Contains(bestDev, "crypt"),
		bestFS == "ecryptfs",
		strings.HasPrefix(bestFS, "fuse."),
		strings.Contains(bestOpts, "fscrypt"),
		strings.Contains(bestOpts, "encrypt"):
		return true, note
	default:
		return false, note
	}
}

func unescapeMount(s string) string {
	r := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return r.Replace(s)
}
