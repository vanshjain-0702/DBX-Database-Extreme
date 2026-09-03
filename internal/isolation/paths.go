package isolation

import "path/filepath"

// Socket names live inside the tenant directory so Landlock and POSIX
// permissions apply to them automatically.
const (
	RESPSocketName = "resp.sock"
	HTTPSocketName = "http.sock"
	ReplSocketName = "repl.sock"
	ACLFileName    = "acl.json"
)

func RESPSocket(dataDir string) string { return filepath.Join(dataDir, RESPSocketName) }
func HTTPSocket(dataDir string) string { return filepath.Join(dataDir, HTTPSocketName) }
func ReplSocket(dataDir string) string { return filepath.Join(dataDir, ReplSocketName) }
func ACLFile(dataDir string) string    { return filepath.Join(dataDir, ACLFileName) }
