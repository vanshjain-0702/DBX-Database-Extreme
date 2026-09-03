package isolation

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
)

// Socket names live inside the tenant directory when the path fits in
// sockaddr_un. Darwin's sun_path is 104 bytes; a GitHub Actions TempDir plus
// /tenants/<id>/http.sock does not. Overflow sockets go under a hashed
// directory in TempDir so Listen does not return "bind: invalid argument".
const (
	RESPSocketName = "resp.sock"
	HTTPSocketName = "http.sock"
	ReplSocketName = "repl.sock"
	ACLFileName    = "acl.json"
)

func unixPathLimit() int {
	// Include the trailing NUL that sockaddr_un requires.
	if runtime.GOOS == "darwin" {
		return 104
	}
	return 108
}

func socketPath(dataDir, name string) string {
	direct := filepath.Join(dataDir, name)
	if len(direct)+1 <= unixPathLimit() {
		return direct
	}
	sum := sha256.Sum256([]byte(filepath.Clean(dataDir)))
	short := filepath.Join(os.TempDir(), "dbx-"+hex.EncodeToString(sum[:8]))
	_ = os.MkdirAll(short, 0o700)
	return filepath.Join(short, name)
}

func RESPSocket(dataDir string) string { return socketPath(dataDir, RESPSocketName) }
func HTTPSocket(dataDir string) string { return socketPath(dataDir, HTTPSocketName) }
func ReplSocket(dataDir string) string { return socketPath(dataDir, ReplSocketName) }
func ACLFile(dataDir string) string    { return filepath.Join(dataDir, ACLFileName) }
