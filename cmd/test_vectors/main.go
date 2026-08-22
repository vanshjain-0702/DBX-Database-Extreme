package main

import (
	"fmt"
	"net"
	"strings"
	"time"
)

func main() {
	conn, err := net.Dial("tcp", "localhost:6399")
	if err != nil {
		fmt.Printf("Failed to connect to DBX: %v\n", err)
		return
	}
	defer conn.Close()

	// Using RESP protocol format to send commands correctly, though inline simple strings work for many servers,
	// RESP array format is safer. We'll send raw strings assuming the parser handles inline commands (which most redis servers do).
	// Let's use RESP arrays to be perfectly safe.
	
	commands := []string{
		"*6\r\n$4\r\nVADD\r\n$5\r\nmyidx\r\n$2\r\nv1\r\n$3\r\n1.0\r\n$3\r\n0.0\r\n$3\r\n0.0\r\n",
		"*6\r\n$4\r\nVADD\r\n$5\r\nmyidx\r\n$2\r\nv2\r\n$3\r\n0.0\r\n$3\r\n1.0\r\n$3\r\n0.0\r\n",
		"*6\r\n$4\r\nVADD\r\n$5\r\nmyidx\r\n$2\r\nv3\r\n$3\r\n0.9\r\n$3\r\n0.1\r\n$3\r\n0.0\r\n",
		"*6\r\n$7\r\nVSEARCH\r\n$5\r\nmyidx\r\n$1\r\n2\r\n$3\r\n1.0\r\n$3\r\n0.0\r\n$3\r\n0.0\r\n",
	}

	for _, cmd := range commands {
		fmt.Printf(">> sending command...\n")
		_, err := conn.Write([]byte(cmd))
		if err != nil {
			fmt.Printf("Write error: %v\n", err)
			return
		}

		time.Sleep(100 * time.Millisecond) // Give server time to respond
		buf := make([]byte, 1024)
		n, _ := conn.Read(buf)
		fmt.Printf("<< %s\n", strings.ReplaceAll(string(buf[:n]), "\r\n", "\\r\\n\n"))
	}
}
