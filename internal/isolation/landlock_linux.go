//go:build linux

package isolation

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	sysLandlockCreateRuleset = 444
	sysLandlockAddRule       = 445
	sysLandlockRestrictSelf  = 446

	landlockCreateRulesetVersion = 1
	landlockRulePathBeneath      = 1

	accessFSExecute    = 1 << 0
	accessFSWriteFile  = 1 << 1
	accessFSReadFile   = 1 << 2
	accessFSReadDir    = 1 << 3
	accessFSRemoveDir  = 1 << 4
	accessFSRemoveFile = 1 << 5
	accessFSMakeChar   = 1 << 6
	accessFSMakeDir    = 1 << 7
	accessFSMakeReg    = 1 << 8
	accessFSMakeSock   = 1 << 9
	accessFSMakeFifo   = 1 << 10
	accessFSMakeBlock  = 1 << 11
	accessFSMakeSym    = 1 << 12
	accessFSRefer      = 1 << 13
	accessFSTruncate   = 1 << 14
)

type landlockRulesetAttr struct {
	handledAccessFS uint64
}

type landlockPathBeneathAttr struct {
	allowedAccess uint64
	parentFd      int32
}

func landlockABI() (int, error) {
	r, _, errno := syscall.Syscall(sysLandlockCreateRuleset, 0, 0, landlockCreateRulesetVersion)
	if errno != 0 {
		return 0, errno
	}
	return int(r), nil
}

func handledAccess(abi int) uint64 {
	access := uint64(accessFSExecute | accessFSWriteFile | accessFSReadFile |
		accessFSReadDir | accessFSRemoveDir | accessFSRemoveFile |
		accessFSMakeChar | accessFSMakeDir | accessFSMakeReg |
		accessFSMakeSock | accessFSMakeFifo | accessFSMakeBlock | accessFSMakeSym)
	if abi >= 2 {
		access |= accessFSRefer
	}
	if abi >= 3 {
		access |= accessFSTruncate
	}
	return access
}

func addPathRule(ruleset int, path string, access uint64) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	attr := landlockPathBeneathAttr{allowedAccess: access, parentFd: int32(dir.Fd())}
	_, _, errno := syscall.Syscall6(sysLandlockAddRule, uintptr(ruleset), landlockRulePathBeneath,
		uintptr(unsafe.Pointer(&attr)), 0, 0, 0)
	if errno != 0 {
		return fmt.Errorf("landlock add_rule %s: %w", path, errno)
	}
	return nil
}

// RestrictFilesystem applies Linux Landlock so this process can only use the
// tenant directory for general filesystem access. /proc, /dev, and /etc remain
// readable because the Go runtime and TLS roots need them. Sibling tenant
// directories are not reachable.
//
// PR_SET_NO_NEW_PRIVS is per-thread. Go may migrate this goroutine between
// syscalls, and landlock_restrict_self then returns EPERM on the thread that
// never got the bit. Pin the rest of the function to one OS thread.
func RestrictFilesystem(tenantDir string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	abi, err := landlockABI()
	if err != nil {
		return fmt.Errorf("landlock is unavailable: %w", err)
	}
	if abi < 1 {
		return fmt.Errorf("landlock ABI %d is too old", abi)
	}
	handled := handledAccess(abi)
	attr := landlockRulesetAttr{handledAccessFS: handled}
	fd, _, errno := syscall.Syscall(sysLandlockCreateRuleset, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr), 0)
	if errno != 0 {
		return fmt.Errorf("landlock create_ruleset: %w", errno)
	}
	ruleset := int(fd)
	defer syscall.Close(ruleset)

	full := handled
	readOnly := uint64(accessFSReadFile | accessFSReadDir)
	devAccess := uint64(accessFSReadFile | accessFSWriteFile | accessFSReadDir)
	if err := addPathRule(ruleset, tenantDir, full); err != nil {
		return err
	}
	if err := addPathRule(ruleset, "/proc", readOnly); err != nil {
		return err
	}
	if err := addPathRule(ruleset, "/etc", readOnly); err != nil {
		return err
	}
	if err := addPathRule(ruleset, "/dev", devAccess); err != nil {
		return err
	}

	if _, _, errno := syscall.Syscall(syscall.SYS_PRCTL, unixPRSetNoNewPrivs, 1, 0); errno != 0 {
		return fmt.Errorf("prctl PR_SET_NO_NEW_PRIVS: %w", errno)
	}
	if _, _, errno := syscall.Syscall(sysLandlockRestrictSelf, uintptr(ruleset), 0, 0); errno != 0 {
		return fmt.Errorf("landlock restrict_self: %w", errno)
	}
	return nil
}

const unixPRSetNoNewPrivs = 38
