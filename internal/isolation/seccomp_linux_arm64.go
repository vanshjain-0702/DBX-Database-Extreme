//go:build linux

package isolation

const (
	auditArch     = 0xc00000b7 // AUDIT_ARCH_AARCH64
	x32SyscallBit = 0
	sysSeccomp    = 277
	sysSocket     = 198
)
