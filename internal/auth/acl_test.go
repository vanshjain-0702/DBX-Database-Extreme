package auth

import "testing"

func TestReaderCannotMutateStringsOrVectors(t *testing.T) {
	store := NewACLStore()
	store.DisableDefault()
	reader := &User{
		Name: "reader", Enabled: true, Permissions: PermRead, AllowedKeys: []string{"*"},
	}
	store.AddUser(reader)

	for _, cmd := range []string{"SET", "SETEX", "DEL", "VADD", "VADD_BATCH", "VDEL", "EXPIRE"} {
		if store.CanExecute(reader, cmd) {
			t.Fatalf("reader must not run %s", cmd)
		}
	}
	for _, cmd := range []string{"GET", "VSEARCH", "TTL", "PING", "MGET"} {
		if !store.CanExecute(reader, cmd) {
			t.Fatalf("reader must run %s", cmd)
		}
	}
	if store.CanExecute(reader, "VCOMPACT") || store.CanExecute(reader, "FLUSHDB") {
		t.Fatal("reader must not run admin commands")
	}
}

func TestWriterCannotCompactOrFlush(t *testing.T) {
	writer := &User{
		Name: "writer", Enabled: true, Permissions: PermRead | PermWrite, AllowedKeys: []string{"*"},
	}
	store := NewACLStore()
	store.DisableDefault()
	if !store.CanExecute(writer, "VADD") || !store.CanExecute(writer, "SET") {
		t.Fatal("writer must ingest")
	}
	if store.CanExecute(writer, "VCOMPACT") || store.CanExecute(writer, "FLUSHALL") {
		t.Fatal("writer is not tenant-admin")
	}
}

func TestTenantAdminCanCompact(t *testing.T) {
	admin := &User{
		Name: "admin", Enabled: true,
		Permissions: PermRead | PermWrite | PermAdmin, AllowedKeys: []string{"*"},
	}
	store := NewACLStore()
	if !store.CanExecute(admin, "VCOMPACT") {
		t.Fatal("tenant-admin must compact")
	}
}
