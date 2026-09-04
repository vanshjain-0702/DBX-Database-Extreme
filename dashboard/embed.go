package dashboard

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed all:dist
var DistFS embed.FS

// Dist returns the Vite build output. Empty until
// `cd dashboard && npm ci && npm run build` (CI and Docker do this before
// `go build`; `make run-dev` / `make build-orchestrator` do it locally).
func Dist() (fs.FS, error) {
	sub, err := fs.Sub(DistFS, "dist")
	if err != nil {
		return nil, err
	}
	f, err := sub.Open("index.html")
	if err != nil {
		return nil, fmt.Errorf("dashboard dist is empty; run: cd dashboard && npm ci && npm run build")
	}
	_ = f.Close()
	return sub, nil
}
