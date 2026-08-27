package protocol

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
)

const (
	maxRESPArrayItems = 4096
	maxRESPBulkBytes  = 8 << 20
	maxRESPLineBytes  = 64 << 10
)

// RESPParser parses RESP2 and inline command format.
type RESPParser struct {
	rd *bufio.Reader
}

// NewRESPParser creates a parser reading from r.
func NewRESPParser(r io.Reader) *RESPParser {
	return &RESPParser{rd: bufio.NewReaderSize(r, 64*1024)}
}

// NewRESPParserFromReader preserves bytes already buffered by a proxy.
func NewRESPParserFromReader(r *bufio.Reader) *RESPParser {
	return &RESPParser{rd: r}
}

func (p *RESPParser) Buffered() int {
	return p.rd.Buffered()
}

// ReadCommand reads and parses the next command from the reader.
func (p *RESPParser) ReadCommand() (*Command, error) {
	b, err := p.rd.ReadByte()
	if err != nil {
		return nil, err
	}
	switch b {
	case '*': // Array (standard RESP)
		return p.readArray()
	case '+', '-', ':', '$': // Inline or partial — treat as inline
		_ = p.rd.UnreadByte()
		return p.readInline()
	default:
		// Inline command
		_ = p.rd.UnreadByte()
		return p.readInline()
	}
}

func (p *RESPParser) readArray() (*Command, error) {
	count, err := p.readInt()
	if err != nil {
		return nil, fmt.Errorf("RESP array count: %w", err)
	}
	if count <= 0 {
		return nil, fmt.Errorf("invalid array count: %d", count)
	}
	if count > maxRESPArrayItems {
		return nil, fmt.Errorf("RESP array count exceeds limit")
	}
	args := make([][]byte, 0, count)
	for i := 0; i < count; i++ {
		bulk, err := p.readBulkString()
		if err != nil {
			return nil, err
		}
		args = append(args, bulk)
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	cmd := &Command{Name: string(args[0])}
	if len(args) > 1 {
		cmd.Args = args[1:]
	}
	return cmd, nil
}

func (p *RESPParser) readBulkString() ([]byte, error) {
	b, err := p.rd.ReadByte()
	if err != nil {
		return nil, err
	}
	if b != '$' {
		return nil, fmt.Errorf("expected bulk string, got %c", b)
	}
	length, err := p.readInt()
	if err != nil {
		return nil, err
	}
	if length < 0 {
		return nil, nil // nil bulk string
	}
	if length > maxRESPBulkBytes {
		return nil, fmt.Errorf("RESP bulk string exceeds limit")
	}
	buf := make([]byte, length+2) // +2 for \r\n
	if _, err = io.ReadFull(p.rd, buf); err != nil {
		return nil, err
	}
	if buf[length] != '\r' || buf[length+1] != '\n' {
		return nil, fmt.Errorf("bulk string missing CRLF terminator")
	}
	return buf[:length], nil
}

func (p *RESPParser) readInt() (int, error) {
	line, err := p.readLine()
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(string(line))
	if err != nil {
		return 0, fmt.Errorf("invalid integer: %s", line)
	}
	return n, nil
}

func (p *RESPParser) readLine() ([]byte, error) {
	var line []byte
	for {
		fragment, isPrefix, err := p.rd.ReadLine()
		if err != nil {
			return nil, err
		}
		line = append(line, fragment...)
		if len(line) > maxRESPLineBytes {
			return nil, fmt.Errorf("RESP line exceeds limit")
		}
		if !isPrefix {
			return bytes.TrimRight(line, "\r\n"), nil
		}
	}
}

func (p *RESPParser) readInline() (*Command, error) {
	line, err := p.readLine()
	if err != nil {
		return nil, err
	}
	parts := bytes.Fields(line)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty inline command")
	}
	cmd := &Command{Name: string(parts[0])}
	for _, p := range parts[1:] {
		arg := make([]byte, len(p))
		copy(arg, p)
		cmd.Args = append(cmd.Args, arg)
	}
	return cmd, nil
}
