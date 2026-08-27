package orchestrator

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestRESPIngressPreservesPipelinedBytes(t *testing.T) {
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	port := backend.Addr().(*net.TCPAddr).Port
	manager, tenant, _ := newTestManager(t)
	tenant.RESPPort = port
	secret, key, err := manager.CreateTenantKey(tenant.ID, "writer", "writer", nil)
	if err != nil {
		t.Fatal(err)
	}
	backendDone := make(chan error, 1)
	go func() {
		conn, err := backend.Accept()
		if err != nil {
			backendDone <- err
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		for i := 0; i < 7; i++ {
			if _, err := reader.ReadString('\n'); err != nil {
				backendDone <- err
				return
			}
		}
		if _, err := conn.Write([]byte("+OK\r\n")); err != nil {
			backendDone <- err
			return
		}
		line, err := reader.ReadString('\n')
		if err == nil && line != "PING\r\n" {
			err = fmt.Errorf("pipelined command = %q", line)
		}
		if err == nil {
			_, err = conn.Write([]byte("+PONG\r\n"))
		}
		backendDone <- err
	}()

	client, ingressSide := net.Pipe()
	defer client.Close()
	go NewRESPIngress(manager, "").handle(ingressSide)
	auth := fmt.Sprintf("*3\r\n$4\r\nAUTH\r\n$%d\r\n%s:%s\r\n$%d\r\n%s\r\nPING\r\n",
		len(tenant.ID)+1+len(key.ID), tenant.ID, key.ID, len(secret), secret)
	if _, err := client.Write([]byte(auth)); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(client)
	first, err := reader.ReadString('\n')
	if err != nil || !strings.HasPrefix(first, "+OK") {
		t.Fatalf("auth response = %q, %v", first, err)
	}
	second, err := reader.ReadString('\n')
	if err != nil || second != "+PONG\r\n" {
		t.Fatalf("pipeline response = %q, %v", second, err)
	}
	if err := <-backendDone; err != nil {
		t.Fatal(err)
	}
}
