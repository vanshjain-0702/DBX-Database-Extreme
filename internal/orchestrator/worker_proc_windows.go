//go:build windows

package orchestrator

import "syscall"

func workerProcAttr() *syscall.SysProcAttr {
	return nil
}
