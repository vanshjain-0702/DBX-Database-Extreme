package isolation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dbx/dbx/internal/security"
)

func TestEnvelopeRoundTripAndShred(t *testing.T) {
	kek, err := GenerateDEK()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DBX_KEK", "") // EnsureDEK takes kek explicitly
	dir := t.TempDir()
	dek, err := EnsureDEK(dir, kek)
	if err != nil {
		t.Fatal(err)
	}
	again, err := UnwrapDEK(dir, kek)
	if err != nil {
		t.Fatal(err)
	}
	if string(dek) != string(again) {
		t.Fatal("unwrapped DEK did not match")
	}
	enc, err := security.NewEncryptor(dek)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "secret.vec")
	if err := WriteSealedFile(path, []byte("embeddings"), enc); err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(onDisk) == "embeddings" {
		t.Fatal("vector file was stored as plaintext")
	}
	plain, err := ReadSealedFile(path, enc)
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != "embeddings" {
		t.Fatalf("got %q", plain)
	}
	if err := ShredDEK(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := UnwrapDEK(dir, kek); err == nil {
		t.Fatal("shredded DEK still unwraps")
	}
}

func TestResolveProfiles(t *testing.T) {
	in := Resolve("")
	if in.Mode != ModeInprocess || in.Process || in.Encryption {
		t.Fatalf("inprocess: %+v", in)
	}
	std := Resolve("standard")
	if !std.Encryption || std.Process {
		t.Fatalf("standard: %+v", std)
	}
	strict := Resolve("strict")
	if !strict.Encryption {
		t.Fatalf("strict must encrypt: %+v", strict)
	}
}
