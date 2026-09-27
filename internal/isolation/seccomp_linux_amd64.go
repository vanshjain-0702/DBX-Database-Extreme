//go:build linux

package isolation

const (
	auditArch     = 0xc000003e // AUDIT_ARCH_X86_64
	x32SyscallBit = 0x40000000
	sysSeccomp    = 317
	sysSocket     = 41
)
