package isolation

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSocketPathFitsUnixLimit(t *testing.T) {
	long := filepath.Join("/var/folders/df/djsxfhc17x95674wsm_g8s980000gn/T",
		"TestStartTenantWithoutCheckoutYAML2893261410", "001", "tenants", "launch")
	got := HTTPSocket(long)
	if len(got)+1 > unixPathLimit() {
		t.Fatalf("socket path still too long (%d): %s", len(got)+1, got)
	}
	if strings.Contains(got, "TestStartTenantWithoutCheckoutYAML") {
		t.Fatalf("expected overflow path, got %s", got)
	}
	short := filepath.Join("data", "tenants", "acme")
	if HTTPSocket(short) != filepath.Join(short, HTTPSocketName) {
		t.Fatalf("short tenant dirs should keep sockets in the data dir")
	}
}
