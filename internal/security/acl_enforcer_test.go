package security

import (
	"strings"
	"testing"

	"github.com/dbx/dbx/internal/auth"
	"github.com/dbx/dbx/internal/protocol"
)

func cmd(name string, args ...string) *protocol.Command {
	c := &protocol.Command{Name: name, Args: make([][]byte, len(args))}
	for i, a := range args {
		c.Args[i] = []byte(a)
	}
	return c
}

func TestEnforceReaderDeniedOnVADDAndSET(t *testing.T) {
	store := auth.NewACLStore()
	store.DisableDefault()
	reader := &auth.User{
		Name: "rk", Enabled: true, Permissions: auth.PermRead, AllowedKeys: []string{"*"},
	}
	store.AddUser(reader)
	enforcer := NewACLEnforcer(store)

	for _, c := range []*protocol.Command{
		cmd("VADD", "memories", "doc:1", "0.1", "0.2"),
		cmd("SET", "session:1", "active"),
		cmd("SETEX", "session:1", "60", "active"),
		cmd("VDEL", "memories", "doc:1"),
	} {
		err := enforcer.Enforce(reader, c)
		if err == nil || !strings.Contains(err.Error(), "NOPERM") {
			t.Fatalf("%s: want NOPERM, got %v", c.Name, err)
		}
	}
	if err := enforcer.Enforce(reader, cmd("VSEARCH", "memories", "0.1", "0.2", "5")); err != nil {
		t.Fatalf("reader VSEARCH: %v", err)
	}
	if err := enforcer.Enforce(reader, cmd("GET", "session:1")); err != nil {
		t.Fatalf("reader GET: %v", err)
	}
}

func TestEnforceKeyPatterns(t *testing.T) {
	store := auth.NewACLStore()
	store.DisableDefault()
	scoped := &auth.User{
		Name: "scoped", Enabled: true,
		Permissions: auth.PermRead | auth.PermWrite,
		AllowedKeys: []string{"agent:*"},
	}
	store.AddUser(scoped)
	enforcer := NewACLEnforcer(store)
	if err := enforcer.Enforce(scoped, cmd("SET", "agent:1", "ok")); err != nil {
		t.Fatal(err)
	}
	err := enforcer.Enforce(scoped, cmd("SET", "other:1", "no"))
	if err == nil || !strings.Contains(err.Error(), "no permissions to access key") {
		t.Fatalf("out-of-scope SET: %v", err)
	}
}

func TestEnforceNilUser(t *testing.T) {
	enforcer := NewACLEnforcer(auth.NewACLStore())
	err := enforcer.Enforce(nil, cmd("GET", "k"))
	if err == nil || !strings.Contains(err.Error(), "NOAUTH") {
		t.Fatalf("got %v", err)
	}
}
