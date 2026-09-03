package isolation

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"time"
)

// IsUnixAddr reports whether addr is a filesystem Unix socket path.
func IsUnixAddr(addr string) bool {
	addr = strings.TrimPrefix(addr, "unix:")
	return strings.HasSuffix(addr, ".sock") || strings.Contains(addr, "/")
}

func unixPath(addr string) string {
	return strings.TrimPrefix(addr, "unix:")
}

// Listen opens a TCP or Unix listener. Unix sockets are 0600 and, on Linux,
// Accept() rejects peers whose PID is not in allowedPIDs when that list is set.
func Listen(addr string, allowedPIDs []int) (net.Listener, error) {
	if !IsUnixAddr(addr) {
		return net.Listen("tcp", addr)
	}
	path := unixPath(addr)
	_ = os.Remove(path)
	if err := os.MkdirAll(parentDir(path), 0o700); err != nil {
		return nil, err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if len(allowedPIDs) == 0 || runtime.GOOS != "linux" {
		return ln, nil
	}
	return &pidListener{Listener: ln, allowed: allowedSet(allowedPIDs)}, nil
}

func parentDir(path string) string {
	dir := path
	for i := len(dir) - 1; i >= 0; i-- {
		if dir[i] == '/' || dir[i] == '\\' {
			if i == 0 {
				return "/"
			}
			return dir[:i]
		}
	}
	return "."
}

func allowedSet(pids []int) map[int]struct{} {
	out := make(map[int]struct{}, len(pids))
	for _, pid := range pids {
		if pid > 0 {
			out[pid] = struct{}{}
		}
	}
	return out
}

type pidListener struct {
	net.Listener
	allowed map[int]struct{}
}

func (l *pidListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		pid, credErr := PeerPID(conn)
		if credErr != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("isolation: peer credentials: %w", credErr)
		}
		if _, ok := l.allowed[pid]; !ok {
			_ = conn.Close()
			continue
		}
		return conn, nil
	}
}

// DialTimeout dials TCP or a Unix socket.
func DialTimeout(addr string, timeout time.Duration) (net.Conn, error) {
	if IsUnixAddr(addr) {
		return net.DialTimeout("unix", unixPath(addr), timeout)
	}
	return net.DialTimeout("tcp", addr, timeout)
}

// Dial dials TCP or a Unix socket.
func Dial(addr string) (net.Conn, error) {
	if IsUnixAddr(addr) {
		return net.Dial("unix", unixPath(addr))
	}
	return net.Dial("tcp", addr)
}
