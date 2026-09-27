//go:build linux && (amd64 || arm64)

package isolation

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	sysSeccompSetModeFilter = 1
	seccompFilterFlagTSync  = 1

	seccompRetKillProcess = 0x80000000
	seccompRetErrno       = 0x00050000
	seccompRetAllow       = 0x7fff0000

	bpfLdWAbs = 0x20 // BPF_LD | BPF_W | BPF_ABS
	bpfJeqK   = 0x15 // BPF_JMP | BPF_JEQ | BPF_K
	bpfJgeK   = 0x35 // BPF_JMP | BPF_JGE | BPF_K
	bpfRetK   = 0x06 // BPF_RET | BPF_K

	seccompDataNr   = 0
	seccompDataArch = 4
	// Low 32 bits of args[0] on little-endian amd64 and arm64.
	seccompDataArg0 = 16

	sysIOURingSetup = 425
)

type sockFilter struct {
	code uint16
	jt   uint8
	jf   uint8
	k    uint32
}

type sockFprog struct {
	len    uint16
	filter *sockFilter
}

// networkFilter denies every socket family except AF_UNIX and io_uring, whose
// ring operations bypass syscall filtering. It rejects syscalls made through a
// foreign ABI (i386, x32) so the socket check cannot be sidestepped.
func networkFilter() []sockFilter {
	const deny = seccompRetErrno | uint32(syscall.EACCES)
	prog := []sockFilter{
		{code: bpfLdWAbs, k: seccompDataArch},
		{code: bpfJeqK, jt: 1, k: auditArch},
		{code: bpfRetK, k: seccompRetKillProcess},
		{code: bpfLdWAbs, k: seccompDataNr},
	}
	if x32SyscallBit != 0 {
		prog = append(prog, sockFilter{code: bpfJgeK, jt: 5, k: x32SyscallBit})
	}
	return append(prog,
		sockFilter{code: bpfJeqK, jt: 4, k: sysIOURingSetup},
		sockFilter{code: bpfJeqK, jf: 2, k: sysSocket},
		sockFilter{code: bpfLdWAbs, k: seccompDataArg0},
		sockFilter{code: bpfJeqK, jf: 1, k: syscall.AF_UNIX},
		sockFilter{code: bpfRetK, k: seccompRetAllow},
		sockFilter{code: bpfRetK, k: deny},
	)
}

// RestrictNetwork installs a seccomp filter on every thread so the worker can
// only open Unix sockets. Listeners bound before this call keep working.
func RestrictNetwork() error {
	if err := setNoNewPrivsAllThreads(); err != nil {
		return err
	}
	filter := networkFilter()
	prog := sockFprog{len: uint16(len(filter)), filter: &filter[0]}
	r, _, errno := syscall.Syscall(sysSeccomp, sysSeccompSetModeFilter, seccompFilterFlagTSync, uintptr(unsafe.Pointer(&prog)))
	if errno != 0 {
		return fmt.Errorf("seccomp network filter: %w", errno)
	}
	if r != 0 {
		return fmt.Errorf("seccomp network filter: thread %d could not be synchronized", r)
	}
	return nil
}

// DisableDumping marks the process non-dumpable so other processes running as
// the same uid (sibling workers) cannot read its memory, environ, or fds
// through /proc or attach with ptrace.
func DisableDumping() error {
	if _, _, errno := syscall.Syscall(syscall.SYS_PRCTL, unixPRSetDumpable, 0, 0); errno != 0 {
		return fmt.Errorf("prctl PR_SET_DUMPABLE: %w", errno)
	}
	return nil
}

const unixPRSetDumpable = 4
