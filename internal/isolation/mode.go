// Package isolation is DBX's Isolation Kernel: the seals that make a tenant
// a sealed execution domain rather than a key prefix.
//
// Claims in this package must stay literally true. The production Linux
// profile is ModeStrict. Density tests and Windows/macOS use weaker profiles
// and must not be described as the security USP.
package isolation

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// Mode is the isolation profile for tenant engines.
type Mode string

const (
	// ModeInprocess keeps today's density-oriented model: one orchestrator
	// process, directory isolation, ACLs, and quotas. It is not the security USP.
	ModeInprocess Mode = "inprocess"
	// ModeStandard adds per-tenant envelope encryption and Unix sockets with
	// kernel peer-credential checks. Engines still share the orchestrator process.
	ModeStandard Mode = "standard"
	// ModeStrict is the Linux production profile: a dedicated worker process,
	// Landlock LSM, cgroup v2, envelope encryption, and Unix sockets that only
	// the orchestrator PID can open.
	ModeStrict Mode = "strict"
)

// Profile is the resolved set of seals for one node.
type Profile struct {
	Mode       Mode
	Encryption bool
	UnixIPC    bool
	PeerCred   bool
	Process    bool
	Landlock   bool
	Cgroup     bool
}

// FromEnv resolves the isolation profile. Unset or unknown values mean
// inprocess so tests, soak, and Windows CI keep their current shape.
func FromEnv() Profile {
	return Resolve(os.Getenv("DBX_ISOLATION_MODE"))
}

// Resolve maps a mode name onto the seals that can actually be applied here.
func Resolve(name string) Profile {
	mode := Mode(strings.ToLower(strings.TrimSpace(name)))
	switch mode {
	case ModeStandard, ModeStrict:
	default:
		mode = ModeInprocess
	}
	p := Profile{Mode: mode}
	switch mode {
	case ModeStandard:
		p.Encryption = true
		p.UnixIPC = unixIPCAvailable()
		p.PeerCred = runtime.GOOS == "linux" && p.UnixIPC
	case ModeStrict:
		p.Encryption = true
		p.UnixIPC = unixIPCAvailable()
		p.PeerCred = runtime.GOOS == "linux" && p.UnixIPC
		if runtime.GOOS == "linux" {
			p.Process = true
			p.Landlock = true
			p.Cgroup = true
		} else {
			// Strict is Linux-only. Other kernels get the standard seals.
			p.Mode = ModeStandard
		}
	}
	return p
}

func unixIPCAvailable() bool {
	switch runtime.GOOS {
	case "linux", "darwin", "freebsd":
		return true
	default:
		return false
	}
}

// UnixAvailable reports whether this kernel can bind filesystem Unix sockets.
func UnixAvailable() bool { return unixIPCAvailable() }

// RequireKEK reports whether this profile must have a wrapping key.
func (p Profile) RequireKEK() bool {
	return p.Encryption
}

func (p Profile) String() string {
	return fmt.Sprintf("mode=%s encrypt=%t unix=%t peercred=%t process=%t landlock=%t cgroup=%t",
		p.Mode, p.Encryption, p.UnixIPC, p.PeerCred, p.Process, p.Landlock, p.Cgroup)
}
