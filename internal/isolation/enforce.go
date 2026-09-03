package isolation

import (
	"fmt"
	"os"
	"strings"
)

// Startup is how the orchestrator was launched. Tests, soak, and
// `make run-dev` (`-insecure-http`) are not production.
type Startup struct {
	Production            bool
	AllowInprocess        bool
	RequireDiskEncryption bool
	DataDir               string
}

// StartupFromEnv builds Startup from flags and environment.
// Production is TLS-on control plane, or DBX_PRODUCTION=1 (Helm/Compose).
func StartupFromEnv(insecureHTTP bool, dataDir string) Startup {
	prod := !insecureHTTP || envTruthy("DBX_PRODUCTION")
	return Startup{
		Production:            prod,
		AllowInprocess:        envTruthy("DBX_ALLOW_INPROCESS"),
		RequireDiskEncryption: envTruthy("DBX_REQUIRE_DISK_ENCRYPTION"),
		DataDir:               dataDir,
	}
}

func envTruthy(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// Enforce fails closed when the process claims to be production but the
// Isolation Kernel is off, or when a sealing profile is missing its KEK.
// inprocess stays legal for tests and `-insecure-http` so density CI does
// not change shape.
func Enforce(p Profile, s Startup) error {
	if p.RequireKEK() {
		if _, err := LoadKEK(); err != nil {
			return fmt.Errorf("%w (required for isolation mode %s)", err, p.Mode)
		}
	}
	if s.Production && p.Mode == ModeInprocess && !s.AllowInprocess {
		return fmt.Errorf("isolation: production refuses inprocess — set DBX_ISOLATION_MODE=strict (Linux) or standard, or DBX_ALLOW_INPROCESS=1 if you accept directory isolation")
	}
	if s.RequireDiskEncryption && s.DataDir != "" {
		ok, note := DataDirLooksEncrypted(s.DataDir)
		if !ok {
			return fmt.Errorf("isolation: DBX_REQUIRE_DISK_ENCRYPTION=1 but %s is not on an encrypted mount (%s); .vec rows are otherwise plaintext", s.DataDir, note)
		}
	}
	return nil
}

// Banner is the one-line boot message operators should see.
func Banner(p Profile, s Startup) string {
	switch {
	case p.Mode == ModeInprocess:
		return fmt.Sprintf("isolation %s — directory + ACL only; this is not the security USP", p)
	case s.Production && p.Mode == ModeStrict:
		return fmt.Sprintf("isolation %s — Isolation Kernel is on", p)
	default:
		return fmt.Sprintf("isolation %s", p)
	}
}

// DiskEncryptionWarning is empty when the data dir looks sealed at rest.
func DiskEncryptionWarning(dataDir string) string {
	if dataDir == "" {
		return ""
	}
	ok, note := DataDirLooksEncrypted(dataDir)
	if ok {
		return ""
	}
	return fmt.Sprintf("WARNING: data dir %s does not look disk-encrypted (%s). Isolation Kernel encrypts WAL/meta/graph; .vec rows stay plaintext unless the volume is LUKS/fscrypt", dataDir, note)
}
