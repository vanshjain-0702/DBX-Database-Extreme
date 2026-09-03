//go:build linux

package isolation

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRestrictFilesystemBlocksSibling(t *testing.T) {
	if os.Getenv("DBX_ISOLATION_HELPER") == "landlock" {
		dir := os.Getenv("DBX_ISOLATION_DIR")
		outside := os.Getenv("DBX_ISOLATION_OUTSIDE")
		if err := RestrictFilesystem(dir); err != nil {
			os.Stderr.WriteString(err.Error())
			os.Exit(2)
		}
		if _, err := os.ReadFile(filepath.Join(dir, "inside")); err != nil {
			os.Exit(3)
		}
		if _, err := os.ReadFile(outside); err == nil {
			os.Exit(4)
		}
		os.Exit(0)
	}

	root := t.TempDir()
	inside := filepath.Join(root, "tenant")
	if err := os.Mkdir(inside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inside, "inside"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "other")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestRestrictFilesystemBlocksSibling")
	cmd.Env = append(os.Environ(),
		"DBX_ISOLATION_HELPER=landlock",
		"DBX_ISOLATION_DIR="+inside,
		"DBX_ISOLATION_OUTSIDE="+outside,
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return
	}
	if exit, ok := err.(*exec.ExitError); ok {
		switch exit.ExitCode() {
		case 2:
			t.Skipf("landlock unavailable: %s", out)
		case 3:
			t.Fatalf("tenant dir became unreadable: %s", out)
		case 4:
			t.Fatal("landlock allowed a sibling path")
		}
	}
	t.Fatalf("helper: %v %s", err, out)
}
