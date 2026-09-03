//go:build unix

package orchestrator

import "syscall"

func workerProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
