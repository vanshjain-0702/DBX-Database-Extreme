package protocol

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestWriterEncodesRESPFamilies(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.WriteOK(); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteSimpleString("PONG"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteInteger(7); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteNull(); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteBulkString(nil); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteBulkStringStr("ab"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteArray(-1); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteStrings([]string{"x", "y"}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteBytes([][]byte{[]byte("z"), nil}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFloat(1.5); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteError("boom"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteErrorRaw("NOAUTH x"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteRaw([]byte(":1\r\n")); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"+OK", "+PONG", ":7", "$-1", "$2", "ab", "*-1", "*2", "1.5", "-ERR boom", "-NOAUTH x"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestCommandRegistryAndHelpers(t *testing.T) {
	cmd := &Command{Name: "get"}
	if cmd.Normalized() != "GET" || cmd.NumArgs() != 0 || cmd.Arg(0) != "" || cmd.ArgBytes(0) != nil {
		t.Fatalf("empty command = %#v", cmd)
	}
	cmd.Args = [][]byte{[]byte("k")}
	if cmd.Arg(0) != "k" || string(cmd.ArgBytes(0)) != "k" {
		t.Fatal("arg accessors")
	}
	info, ok := Lookup("set")
	if !ok || !info.DurableV1 || info.ReadOnly {
		t.Fatalf("SET info = %#v", info)
	}
	if !SupportedInDurableV1("GET") || !SupportedInDurableV1("SET") || SupportedInDurableV1("HSET") {
		t.Fatal("durable profile classification")
	}
	if !LookupMustReadOnly("GET") || LookupMustReadOnly("SET") {
		t.Fatal("read-only lookup")
	}
	if ShouldAudit("SET") || !ShouldAudit("FLUSHALL") {
		t.Fatal("audit classification")
	}
	if got := WrongNumArgsError("GET"); !strings.Contains(got, "GET") {
		t.Fatal(got)
	}
	err := ErrProtocol("bad %d", 1)
	if err.Error() != "bad 1" {
		t.Fatal(err)
	}
}

func TestAffectedKeysAndInlineParser(t *testing.T) {
	if keys := AffectedKeys(&Command{Name: "DEL", Args: [][]byte{[]byte("a"), []byte("b")}}); len(keys) != 2 {
		t.Fatal(keys)
	}
	if keys := AffectedKeys(&Command{Name: "PING"}); keys != nil {
		t.Fatal(keys)
	}
	parser := NewRESPParserFromReader(bufio.NewReader(strings.NewReader("PING\r\n")))
	cmd, err := parser.ReadCommand()
	if err != nil || cmd.Normalized() != "PING" {
		t.Fatalf("%v %#v", err, cmd)
	}
	if parser.Buffered() < 0 {
		t.Fatal("buffered")
	}
}
