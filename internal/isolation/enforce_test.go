package isolation

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestEnforceAllowsInprocessOutsideProduction(t *testing.T) {
	t.Setenv("DBX_KEK", "")
	p := Resolve("")
	if err := Enforce(p, Startup{Production: false}); err != nil {
		t.Fatal(err)
	}
}

func TestEnforceRefusesInprocessInProduction(t *testing.T) {
	t.Setenv("DBX_KEK", "")
	p := Resolve("")
	err := Enforce(p, Startup{Production: true})
	if err == nil || !strings.Contains(err.Error(), "refuses inprocess") {
		t.Fatalf("got %v", err)
	}
	if err := Enforce(p, Startup{Production: true, AllowInprocess: true}); err != nil {
		t.Fatal(err)
	}
}

func TestEnforceRequiresKEKForSealedProfiles(t *testing.T) {
	t.Setenv("DBX_KEK", "")
	err := Enforce(Resolve("standard"), Startup{Production: false})
	if err == nil || !strings.Contains(err.Error(), "DBX_KEK") {
		t.Fatalf("got %v", err)
	}
	t.Setenv("DBX_KEK", hex.EncodeToString(make([]byte, 32)))
	if err := Enforce(Resolve("standard"), Startup{Production: false}); err != nil {
		t.Fatal(err)
	}
}

func TestStartupFromEnv(t *testing.T) {
	t.Setenv("DBX_PRODUCTION", "")
	t.Setenv("DBX_ALLOW_INPROCESS", "")
	t.Setenv("DBX_REQUIRE_DISK_ENCRYPTION", "")
	dev := StartupFromEnv(true, "/data")
	if dev.Production {
		t.Fatal("insecure-http is not production")
	}
	t.Setenv("DBX_PRODUCTION", "1")
	prod := StartupFromEnv(true, "/data")
	if !prod.Production {
		t.Fatal("DBX_PRODUCTION=1 must force production even with insecure-http")
	}
	if StartupFromEnv(false, "/data").Production != true {
		t.Fatal("TLS control plane is production")
	}
}

func TestBannerNamesTheUSP(t *testing.T) {
	msg := Banner(Resolve(""), Startup{})
	if !strings.Contains(msg, "not the security USP") {
		t.Fatalf("%q", msg)
	}
}
