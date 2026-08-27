package orchestrator

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/dbx/dbx/internal/protocol"
)

// RESPIngress is the single authenticated public data-plane front door.
type RESPIngress struct {
	manager *Manager
	addr    string
}

func NewRESPIngress(manager *Manager, addr string) *RESPIngress {
	return &RESPIngress{manager: manager, addr: addr}
}

func (p *RESPIngress) ListenAndServe(ctx context.Context) error {
	listener, err := net.Listen("tcp", p.addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	go func() {
		<-ctx.Done()
		listener.Close()
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			continue
		}
		go p.handle(conn)
	}
}

func (p *RESPIngress) handle(client net.Conn) {
	defer client.Close()
	defer func() {
		_ = recover() // isolate malformed-client and proxy panics
	}()
	_ = client.SetDeadline(time.Now().Add(10 * time.Second))
	reader := bufio.NewReaderSize(client, 64*1024)
	parser := protocol.NewRESPParserFromReader(reader)
	cmd, err := parser.ReadCommand()
	if err != nil || cmd.Normalized() != "AUTH" || cmd.NumArgs() != 2 {
		_, _ = client.Write([]byte("-NOAUTH initial AUTH <tenant>:<key-id> <secret> required\r\n"))
		return
	}
	identity := strings.SplitN(cmd.Arg(0), ":", 2)
	if len(identity) != 2 || len(identity[0]) > 128 || len(identity[1]) > 128 || len(cmd.Arg(1)) > 256 {
		_, _ = client.Write([]byte("-WRONGPASS invalid tenant credential\r\n"))
		return
	}
	tenant, ok := p.manager.VerifyTenantKey(identity[0], identity[1], cmd.Arg(1))
	if !ok {
		_, _ = client.Write([]byte("-WRONGPASS invalid tenant credential\r\n"))
		return
	}
	backend, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", tenant.RESPPort), 5*time.Second)
	if err != nil {
		_, _ = client.Write([]byte("-ERR tenant unavailable\r\n"))
		return
	}
	defer backend.Close()
	_ = backend.SetDeadline(time.Now().Add(10 * time.Second))
	secret := cmd.Arg(1)
	if _, err := fmt.Fprintf(backend, "*3\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n",
		len(identity[1]), identity[1], len(secret), secret); err != nil {
		return
	}
	backendReader := bufio.NewReaderSize(backend, 64*1024)
	authResponse, err := backendReader.ReadString('\n')
	if err != nil || !strings.HasPrefix(authResponse, "+OK") {
		_, _ = client.Write([]byte("-WRONGPASS invalid tenant credential\r\n"))
		return
	}
	if _, err := client.Write([]byte("+OK\r\n")); err != nil {
		return
	}
	_ = client.SetDeadline(time.Time{})
	_ = backend.SetDeadline(time.Time{})

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(backend, reader)
		if tcp, ok := backend.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, backendReader)
		if tcp, ok := client.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		done <- struct{}{}
	}()
	<-done
}
