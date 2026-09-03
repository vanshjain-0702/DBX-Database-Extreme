package isolation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSocketPathFitsSockaddrUn(t *testing.T) {
	long := filepath.Join(t.TempDir(), strings.Repeat("tenant", 20), "nested")
	if err := os.MkdirAll(long, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{RESPSocket(long), HTTPSocket(long), ReplSocket(long)} {
		if len(p)+1 > unixPathLimit() {
			t.Fatalf("socket path %q is %d bytes, limit %d", p, len(p)+1, unixPathLimit())
		}
	}
}
