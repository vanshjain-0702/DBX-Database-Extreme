//go:build linux && (amd64 || arm64)

package isolation

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

const (
	helperExitOK          = 0
	helperExitUnavailable = 2
	helperExitCgo         = 5
	helperExitLeak        = 6
)

// runLockdownHelper re-executes the test binary with the named helper and maps
// its exit status to pass, skip, or fail.
func runLockdownHelper(t *testing.T, name string, env ...string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$")
	cmd.Env = append(append(os.Environ(), "DBX_LOCKDOWN_HELPER="+name), env...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		switch exit.ExitCode() {
		case helperExitUnavailable:
			t.Skipf("kernel seal unavailable here: %s", out)
		case helperExitCgo:
			t.Skipf("test binary links cgo (e.g. -race); workers are built with CGO_ENABLED=0: %s", out)
		case helperExitLeak:
			t.Fatalf("seal leaked: %s", out)
		}
	}
	t.Fatalf("helper %s: %v %s", name, err, out)
}

func helperFail(code int, format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(code)
}

func sealErrorExit(err error) {
	if errors.Is(err, ErrThreadSealUnsupported) {
		helperFail(helperExitCgo, "%v", err)
	}
	helperFail(helperExitUnavailable, "%v", err)
}

// pinnedThreads starts n goroutines that each own an OS thread before the seal
// is applied, then runs probe on every one of them after release is closed.
func pinnedThreads(n int, probe func() error) (release func() []error) {
	ready := make(chan struct{})
	start := make(chan struct{})
	results := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			runtime.LockOSThread()
			ready <- struct{}{}
			<-start
			results <- probe()
		}()
	}
	for i := 0; i < n; i++ {
		<-ready
	}
	return func() []error {
		close(start)
		out := make([]error, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, <-results)
		}
		return out
	}
}

// Landlock and no_new_privs are per-thread. A goroutine that already owned a
// separate OS thread when the seal went on must be confined too.
func TestRestrictFilesystemCoversPinnedThreads(t *testing.T) {
	if os.Getenv("DBX_LOCKDOWN_HELPER") == "pinned-fs" {
		runtime.GOMAXPROCS(8)
		outside := os.Getenv("DBX_LOCKDOWN_OUTSIDE")
		release := pinnedThreads(16, func() error {
			if _, err := os.ReadFile(outside); err == nil {
				return errors.New("pinned thread read a file outside the tenant directory")
			}
			return nil
		})
		if err := RestrictFilesystem(os.Getenv("DBX_LOCKDOWN_DIR")); err != nil {
			sealErrorExit(err)
		}
		for _, err := range release() {
			if err != nil {
				helperFail(helperExitLeak, "%v", err)
			}
		}
		os.Exit(helperExitOK)
	}

	root := t.TempDir()
	inside := filepath.Join(root, "tenant")
	if err := os.Mkdir(inside, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "neighbour-dek")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	runLockdownHelper(t, "pinned-fs", "DBX_LOCKDOWN_DIR="+inside, "DBX_LOCKDOWN_OUTSIDE="+outside)
}

// After LockDown a worker may only use Unix sockets, on every thread.
func TestLockDownAllowsOnlyUnixSockets(t *testing.T) {
	if os.Getenv("DBX_LOCKDOWN_HELPER") == "network" {
		dir := os.Getenv("DBX_LOCKDOWN_DIR")
		pre, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			helperFail(1, "pre-bind: %v", err)
		}
		tryNet := func() error {
			if c, err := net.Dial("tcp", pre.Addr().String()); err == nil {
				c.Close()
				return errors.New("TCP connect succeeded after lockdown")
			}
			if l, err := net.Listen("tcp", "127.0.0.1:0"); err == nil {
				l.Close()
				return errors.New("TCP listen succeeded after lockdown")
			}
			if c, err := net.Dial("udp", "127.0.0.1:53"); err == nil {
				c.Close()
				return errors.New("UDP socket created after lockdown")
			}
			if _, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW, 0); err == nil {
				return errors.New("netlink socket created after lockdown")
			}
			return nil
		}
		release := pinnedThreads(8, tryNet)
		if err := LockDown(dir); err != nil {
			sealErrorExit(err)
		}
		for _, err := range append(release(), tryNet()) {
			if err != nil {
				helperFail(helperExitLeak, "%v", err)
			}
		}
		sock := filepath.Join(dir, "ok.sock")
		ln, err := net.Listen("unix", sock)
		if err != nil {
			helperFail(1, "unix listen after lockdown: %v", err)
		}
		ln.Close()
		pre.Close()
		os.Exit(helperExitOK)
	}

	dir := t.TempDir()
	runLockdownHelper(t, "network", "DBX_LOCKDOWN_DIR="+dir)
}

func TestLockDownMakesWorkerNonDumpable(t *testing.T) {
	if os.Getenv("DBX_LOCKDOWN_HELPER") == "dumpable" {
		if err := LockDown(os.Getenv("DBX_LOCKDOWN_DIR")); err != nil {
			sealErrorExit(err)
		}
		const prGetDumpable = 3
		r, _, errno := syscall.Syscall(syscall.SYS_PRCTL, prGetDumpable, 0, 0)
		if errno != 0 {
			helperFail(1, "PR_GET_DUMPABLE: %v", errno)
		}
		if r != 0 {
			helperFail(helperExitLeak, "worker is still dumpable (%d)", r)
		}
		os.Exit(helperExitOK)
	}
	runLockdownHelper(t, "dumpable", "DBX_LOCKDOWN_DIR="+t.TempDir())
}

func TestNetworkFilterJumpsStayInBounds(t *testing.T) {
	prog := networkFilter()
	for i, ins := range prog {
		if ins.code&0x07 != 0x05 { // BPF_JMP
			continue
		}
		for _, off := range []uint8{ins.jt, ins.jf} {
			if target := i + 1 + int(off); target >= len(prog) {
				t.Fatalf("instruction %d jumps to %d past the end (%d)", i, target, len(prog))
			}
		}
	}
	last := prog[len(prog)-1]
	if last.code != bpfRetK || last.k&0xffff0000 != seccompRetErrno {
		t.Fatalf("filter must end in the deny return, got %+v", last)
	}
}
