package protocol

import (
	"strings"
	"testing"
)

func TestRESPParserPipeliningAndBinaryPayload(t *testing.T) {
	input := "*3\r\n$3\r\nSET\r\n$1\r\nk\r\n$3\r\na\x00b\r\nPING\r\n"
	parser := NewRESPParser(strings.NewReader(input))
	first, err := parser.ReadCommand()
	if err != nil {
		t.Fatal(err)
	}
	if first.Normalized() != "SET" || string(first.ArgBytes(1)) != "a\x00b" {
		t.Fatalf("unexpected first command: %#v", first)
	}
	second, err := parser.ReadCommand()
	if err != nil || second.Normalized() != "PING" {
		t.Fatalf("unexpected second command: %#v, %v", second, err)
	}
}

func TestRESPParserRejectsMalformedTerminator(t *testing.T) {
	parser := NewRESPParser(strings.NewReader("*2\r\n$3\r\nGET\r\n$1\r\nkx"))
	if _, err := parser.ReadCommand(); err == nil {
		t.Fatal("expected malformed bulk terminator to fail")
	}
}

func TestAffectedKeysCoversMultiKeyCommands(t *testing.T) {
	command := &Command{Name: "MSET", Args: [][]byte{[]byte("a"), []byte("1"), []byte("b"), []byte("2")}}
	keys := AffectedKeys(command)
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "b" {
		t.Fatalf("affected keys = %#v", keys)
	}
}

func FuzzRESPParser(f *testing.F) {
	f.Add([]byte("*1\r\n$4\r\nPING\r\n"))
	f.Add([]byte("SET key value\r\n"))
	f.Fuzz(func(t *testing.T, input []byte) {
		parser := NewRESPParser(strings.NewReader(string(input)))
		_, _ = parser.ReadCommand()
	})
}
